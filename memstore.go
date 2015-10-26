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

package quorum

import (
	"sync"
)

type memStorage struct {
	mu        sync.Mutex
	elections map[string]Election
	ballots   map[string]Ballot
}

// NewMemStorage returns a new ephemeral in-memory Storage implementation.
func NewMemStorage() *memStorage {
	return &memStorage{
		elections: map[string]Election{},
		ballots:   map[string]Ballot{},
	}
}

// Add implements the Storage interface.
func (s *memStorage) Add(election Election, ballots []Ballot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.elections[election.ID] = election
	for _, ballot := range ballots {
		s.ballots[ballot.ID] = ballot
	}
	return nil
}

// Approve implements the Storage interface.
func (s *memStorage) Approve(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ballot, ok := s.ballots[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.ballots, id)

	election, ok := s.elections[ballot.Election]
	if !ok {
		return ErrNotFound
	}
	election.NApprovals++
	s.elections[ballot.Election] = election
	return nil
}

// Deny implements the Storage interface.
func (s *memStorage) Deny(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ballot, ok := s.ballots[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.ballots, id)

	election, ok := s.elections[ballot.Election]
	if !ok {
		return ErrNotFound
	}
	election.NDenials++
	s.elections[ballot.Election] = election
	return nil
}

// Election implements the Storage interface.
func (s *memStorage) Election(id string) (Election, error) {
	var fail Election
	s.mu.Lock()
	defer s.mu.Unlock()
	election, ok := s.elections[id]
	if !ok {
		return fail, ErrNotFound
	}
	return election, nil
}

// Close implements the Storage interface.
func (s *memStorage) Close(id string) error {
	delete(s.elections, id)
	return nil
}
