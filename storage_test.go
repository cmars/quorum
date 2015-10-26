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

package quorum_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/cmars/quorum"
)

func Test(t *testing.T) { gc.TestingT(t) }

type StorageSuite struct {
	store quorum.Storage
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) SetUpTest(c *gc.C) {
	s.store = quorum.NewMemStorage()
}
func (s *StorageSuite) TestDifferentiateElections(c *gc.C) {
	election1 := quorum.Election{
		Policy: quorum.Policy{
			NApprovalsRequired: 1,
			Participants:       []string{"alice@e1", "bob@e1"},
			Message:            "election1 message",
		},
		ID:       "election1-id",
		CaveatID: "election1-caveatid",
	}
	err := s.store.Add(election1, []quorum.Ballot{{
		ID:        "alice-ballot",
		Election:  "election1-id",
		Recipient: "alice@e1",
	}, {
		ID:        "bob-ballot",
		Election:  "election1-id",
		Recipient: "bob@e1",
	}})
	c.Assert(err, gc.IsNil)

	election2 := quorum.Election{
		Policy: quorum.Policy{
			NApprovalsRequired: 2,
			Participants:       []string{"carol@e2", "dave@e2"},
			Message:            "election2 message",
		},
		ID:       "election2-id",
		CaveatID: "election2-caveatid",
	}
	err = s.store.Add(election2, []quorum.Ballot{{
		ID:        "carol-ballot",
		Election:  "election2-id",
		Recipient: "carol@e2",
	}, {
		ID:        "dave-ballot",
		Election:  "election2-id",
		Recipient: "dave@e2",
	}})
	c.Assert(err, gc.IsNil)

	election1Resp, err := s.store.Election("election1-id")
	c.Assert(err, gc.IsNil)
	c.Assert(election1Resp, gc.DeepEquals, election1)

	election2Resp, err := s.store.Election("election2-id")
	c.Assert(err, gc.IsNil)
	c.Assert(election2Resp, gc.DeepEquals, election2)
}

func (s *StorageSuite) TestInvalidElection(c *gc.C) {
	election := quorum.Election{
		Policy: quorum.Policy{
			NApprovalsRequired: 3,
			Participants:       []string{"alice@e1", "bob@e1"},
			Message:            "election message",
		},
		ID:       "election-id",
		CaveatID: "election-caveatid",
	}
	err := election.Validate()
	c.Assert(err, gc.ErrorMatches, "2 recipients will never satisfy 3 approver requirement")
}

func (s *StorageSuite) TestAccept(c *gc.C) {
	election := quorum.Election{
		Policy: quorum.Policy{
			NApprovalsRequired: 2,
			Participants:       []string{"alice@e1", "bob@e1"},
			Message:            "election message",
		},
		ID:       "election-id",
		CaveatID: "election-caveatid",
	}
	err := s.store.Add(election, []quorum.Ballot{{
		ID:        "alice-ballot",
		Election:  "election-id",
		Recipient: "alice@e1",
	}, {
		ID:        "bob-ballot",
		Election:  "election-id",
		Recipient: "bob@e1",
	}})
	c.Assert(err, gc.IsNil)
	s.assertResult(c, "election-id", quorum.ElectionPending)

	err = s.store.Approve("alice-ballot")
	c.Assert(err, gc.IsNil)
	s.assertResult(c, "election-id", quorum.ElectionPending)

	err = s.store.Approve("alice-ballot")
	c.Assert(err, gc.ErrorMatches, "not found")
	s.assertResult(c, "election-id", quorum.ElectionPending)

	err = s.store.Approve("mallory-ballot")
	c.Assert(err, gc.ErrorMatches, "not found")
	s.assertResult(c, "election-id", quorum.ElectionPending)

	err = s.store.Deny("mallory-ballot")
	c.Assert(err, gc.ErrorMatches, "not found")
	s.assertResult(c, "election-id", quorum.ElectionPending)

	err = s.store.Approve("bob-ballot")
	c.Assert(err, gc.IsNil)
	s.assertResult(c, "election-id", quorum.ElectionApproved)
}

func (s *StorageSuite) assertResult(c *gc.C, id string, result quorum.ElectionResult) {
	e, err := s.store.Election(id)
	c.Assert(err, gc.IsNil)
	c.Assert(e.Result(), gc.Equals, result)
}

func (s *StorageSuite) TestDeny(c *gc.C) {
	election := quorum.Election{
		Policy: quorum.Policy{
			NApprovalsRequired: 2,
			Participants:       []string{"alice@e1", "bob@e1"},
			Message:            "election message",
		},
		ID:       "election-id",
		CaveatID: "election-caveatid",
	}
	err := s.store.Add(election, []quorum.Ballot{{
		ID:        "alice-ballot",
		Election:  "election-id",
		Recipient: "alice@e1",
	}, {
		ID:        "bob-ballot",
		Election:  "election-id",
		Recipient: "bob@e1",
	}})
	c.Assert(err, gc.IsNil)
	s.assertResult(c, "election-id", quorum.ElectionPending)

	err = s.store.Deny("alice-ballot")
	c.Assert(err, gc.IsNil)
	s.assertResult(c, "election-id", quorum.ElectionDenied)

	err = s.store.Approve("alice-ballot")
	c.Assert(err, gc.ErrorMatches, "not found")
	s.assertResult(c, "election-id", quorum.ElectionDenied)

	err = s.store.Approve("mallory-ballot")
	c.Assert(err, gc.ErrorMatches, "not found")
	s.assertResult(c, "election-id", quorum.ElectionDenied)

	err = s.store.Deny("mallory-ballot")
	c.Assert(err, gc.ErrorMatches, "not found")
	s.assertResult(c, "election-id", quorum.ElectionDenied)

	// Bob can approve, but it won't change the outcome.
	err = s.store.Approve("bob-ballot")
	c.Assert(err, gc.IsNil)
	s.assertResult(c, "election-id", quorum.ElectionDenied)
}

func (s *StorageSuite) TestNoApprovalsRequired(c *gc.C) {
	election := quorum.Election{
		Policy: quorum.Policy{
			NApprovalsRequired: 0,
			Participants:       []string{"alice@e1", "bob@e1"},
			Message:            "election message",
		},
		ID:       "election-id",
		CaveatID: "election-caveatid",
	}
	err := s.store.Add(election, []quorum.Ballot{{
		ID:        "alice-ballot",
		Election:  "election-id",
		Recipient: "alice@e1",
	}, {
		ID:        "bob-ballot",
		Election:  "election-id",
		Recipient: "bob@e1",
	}})
	c.Assert(err, gc.IsNil)
	s.assertResult(c, "election-id", quorum.ElectionApproved)
}
