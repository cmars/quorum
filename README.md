# quorum

This service discharges third-party caveats that require a quorum of
participants to approve a request.

When a request for a discharge is made by a client, an election is started in
which each participant is contacted to approve or deny (veto) its issuance. The
client is directed to a URL where it may retry the request until a quorum is
reached and the dischared is issued, the request is denied, or the election
expires due to inactivity of the participants beyond a time limit.

# License

Copyright 2015 Casey Marshall.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
