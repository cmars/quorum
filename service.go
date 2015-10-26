/*
 * Copyright 2015 Casey Marshall
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package quorum provides a service that discharges third-party caveats that
// require a quorum of participants to approve a request.
package quorum

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

// Service is the quorum caveat discharging service.
type Service struct {
	bakery *bakery.Service
	mux    *http.ServeMux
	sender Sender
	store  Storage

	root, prefix string
}

// ServiceConfig is used to configure a new Service.
type ServiceConfig struct {
	Prefix string
}

// Policy defines what constitutes a quorum of approval, and is encoded as the
// third-party caveat condition to the quorum service.
type Policy struct {
	NApprovalsRequired int
	Participants       []string
	Message            string
}

// Validate returns an error if the Policy is invalid. For example, if the
// Policy cannot ever be satisfied.
func (p *Policy) Validate() error {
	if len(p.Participants) == 0 {
		return errgo.Newf("no recipients specified")
	}
	if len(p.Participants) < p.NApprovalsRequired {
		return errgo.Newf("%d recipients will never satisfy %d approver requirement",
			len(p.Participants), p.NApprovalsRequired)
	}
	return nil
}

// Election represents an active election in which a quorum is sought for
// a discharge request.
type Election struct {
	Policy
	ID       string
	CaveatID string

	NApprovals int
	NDenials   int
}

// ElectionResult describes the current outcome of the election.
type ElectionResult string

const (
	ElectionPending  = ElectionResult("pending")
	ElectionApproved = ElectionResult("approved")
	ElectionDenied   = ElectionResult("denied")
)

// Result returns the current election result.
func (e *Election) Result() ElectionResult {
	if e.NDenials > 0 {
		return ElectionDenied
	}
	if e.NApprovals >= e.NApprovalsRequired {
		return ElectionApproved
	}
	return ElectionPending
}

// Ballot is used to track a particpant's response in an election.
type Ballot struct {
	ID        string
	Election  string
	Recipient string
	Message   string
	Used      bool
}

// ErrNotFound indicates a storage lookup did not match.
var ErrNotFound = errgo.New("not found")

// Storage defines the interface for persisting elections across requests.
type Storage interface {
	Add(election Election, ballots []Ballot) error
	Approve(ballot string) error
	Deny(ballot string) error
	Election(id string) (Election, error)
	Close(id string) error
}

// Sender defines the interface for validating and contacting participants with
// ballots.
type Sender interface {
	ValidateRecipient(recipient string) error
	Send(ballot Ballot) error
}

// NewService returns a new Service instance.
func NewService(config ServiceConfig) (*Service, error) {
	bakeryService, err := bakery.NewService(bakery.NewServiceParams{})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}

	s := &Service{bakery: bakeryService, prefix: config.Prefix}

	s.mux = http.NewServeMux()
	httpbakery.AddDischargeHandler(s.mux, config.Prefix+"/discharger", s.bakery, s.checker)
	r := httprouter.New()
	r.GET(config.Prefix+"/wait/:election", s.wait)
	r.GET(config.Prefix+"/approve/:ballot", s.approve)
	r.GET(config.Prefix+"/deny/:ballot", s.deny)
	s.mux.Handle("/", r)
	return s, nil
}

// ServeHTTP implements http.Handler.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

const idLen = 32

func newID() (string, error) {
	var fail string
	var buf [idLen]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return fail, err
	}
	return base64.URLEncoding.EncodeToString(buf[:]), nil
}

func (s *Service) checker(req *http.Request, cavID, cav string) ([]checkers.Caveat, error) {
	election, err := s.newElection(cavID, cav)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	var waitURL string
	if s.root != "" {
		waitURL = s.root + s.prefix + "/wait/" + election
	} else {
		waitURL = s.prefix + "/wait/" + election
	}
	return nil, httpbakery.NewInteractionRequiredError(waitURL, waitURL, nil, req)
}

func (s *Service) newElection(cavID, cav string) (string, error) {
	var fail string

	var policy Policy
	err := json.Unmarshal([]byte(cav), &policy)
	if err != nil {
		return fail, errgo.Notef(err, "invalid caveat %q", cav)
	}
	err = policy.Validate()
	if err != nil {
		return fail, errgo.Mask(err, errgo.Any)
	}

	electionID, err := newID()
	if err != nil {
		return fail, errgo.Mask(err)
	}
	election := Election{ID: electionID, CaveatID: cavID, Policy: policy}

	var ballots []Ballot
	for _, recipient := range policy.Participants {
		err := s.sender.ValidateRecipient(recipient)
		if err != nil {
			return fail, errgo.Mask(err)
		}
		ballotID, err := newID()
		if err != nil {
			return fail, errgo.Mask(err)
		}
		ballots = append(ballots, Ballot{
			ID:        ballotID,
			Election:  electionID,
			Recipient: recipient,
			Message:   policy.Message,
		})
	}

	err = s.store.Add(election, ballots)
	if err != nil {
		return fail, errgo.Mask(err, errgo.Any)
	}

	for _, ballot := range ballots {
		err = s.sender.Send(ballot)
		if err != nil {
			log.Println("failed to send ballot to %q: %v", ballot.Recipient, err)
			// TODO: allow some failures depending on the quorum required?
			return fail, errgo.Mask(err, errgo.Any)
		}
	}
	return electionID, nil
}

func httpErrorf(w http.ResponseWriter, statusCode int, err error) {
	http.Error(w, err.Error(), statusCode)
	log.Printf("HTTP %d: %s", statusCode, errgo.Details(err))
}

func (s *Service) wait(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	id := p.ByName("election")
	if id == "" {
		httpErrorf(w, http.StatusBadRequest, errgo.New("missing election param"))
		return
	}
	election, err := s.store.Election(id)
	if err != nil {
		httpErrorf(w, http.StatusBadRequest, errgo.Mask(err, errgo.Any))
		return
	}
	switch election.Result() {
	case ElectionPending:
		w.WriteHeader(http.StatusAccepted)
	case ElectionDenied:
		w.WriteHeader(http.StatusForbidden)
	case ElectionApproved:
		dm, err := s.bakery.Discharge(nil, election.CaveatID)
		if err != nil {
			httpErrorf(w, http.StatusInternalServerError, errgo.Mask(err, errgo.Any))
			return
		}

		err = s.store.Close(election.ID)
		if err != nil {
			httpErrorf(w, http.StatusInternalServerError, errgo.Mask(err, errgo.Any))
			return
		}

		w.WriteHeader(http.StatusCreated)
		enc := json.NewEncoder(w)
		err = enc.Encode(dm)
		if err != nil {
			log.Println("failed to encode macaroon in response: %v", errgo.Mask(err))
		}
	default:
		httpErrorf(w, http.StatusInternalServerError, errgo.Newf("invalid election result %q", election.Result()))
	}
}

func (s *Service) approve(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	ballotID := p.ByName("ballot")
	if ballotID == "" {
		httpErrorf(w, http.StatusBadRequest, errgo.New("missing ballot ID"))
	}
	err := s.store.Approve(ballotID)
	if err != nil {
		httpErrorf(w, http.StatusInternalServerError, errgo.Notef(err, "storage failed on 'approve'"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte("approved"))
}

func (s *Service) deny(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	ballot := p.ByName("ballot")
	if ballot == "" {
		httpErrorf(w, http.StatusBadRequest, errgo.New("missing ballot ID"))
	}
	err := s.store.Deny(ballot)
	if err != nil {
		httpErrorf(w, http.StatusInternalServerError, errgo.Notef(err, "storage failed on 'deny'"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte("denied"))
}
