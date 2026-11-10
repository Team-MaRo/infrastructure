package uploadscan

import (
	"net/http"
	"strconv"
	"strings"
)

// statusClientGone is the status Traefik's proxy gives a client that left.
const statusClientGone = 499

// Retry-After in seconds: soon for memory and an address's places, later for clamd and the scan's
// time.
const (
	retrySoon  = 10
	retryLater = 30
)

// refusal is the answer to a request the scan did not call clean: its status, the seconds after
// which the same upload may pass (0: it would fail again), the reason's code and what its text
// names - the signature, or the size limit in MB (10^6 bytes, rounded down).
type refusal struct {
	status     int
	retryAfter int
	reason     string
	detail     string
}

func final(status int, reason, detail string) *refusal {
	return &refusal{status: status, reason: reason, detail: detail}
}

func temporary(status, retryAfter int, reason string) *refusal {
	return &refusal{status: status, retryAfter: retryAfter, reason: reason}
}

// refuse decides, in block mode, how a request is refused: everything the scan looked at and did
// not call clean is. nil lets it pass - clean, or nothing to scan. What fails again on a retry -
// malware found, an ambiguous body - goes before what may pass later.
func refuse(res result, gone bool) *refusal {
	if res.Verdict == "" || res.Verdict == "clean" && res.Ambiguous == "" {
		return nil
	}
	if gone {
		return &refusal{status: statusClientGone}
	}
	if res.Verdict == "infected" {
		return final(http.StatusUnprocessableEntity, "infected", clip(res.Sig))
	}
	if res.Ambiguous != "" {
		return final(http.StatusUnprocessableEntity, "ambiguous", "")
	}
	switch res.Verdict {
	case "limits-exceeded":
		if timedOut(res.Sig) {
			return temporary(http.StatusServiceUnavailable, retryLater, "scan-time")
		}
		return final(http.StatusUnprocessableEntity, "limits-exceeded", clip(res.Sig))
	case "skipped:too-large":
		mb := strconv.FormatInt(maxBytes/1000000, 10)
		return final(http.StatusRequestEntityTooLarge, "too-large", mb)
	case "skipped:busy":
		return temporary(http.StatusServiceUnavailable, retrySoon, "busy")
	case "skipped:too-many":
		return temporary(http.StatusTooManyRequests, retrySoon, "too-many")
	case "incomplete", "skipped:time":
		return temporary(http.StatusServiceUnavailable, retryLater, "scan-time")
	case "aborted":
		return final(http.StatusBadRequest, "incomplete-body", "")
	}
	return temporary(http.StatusServiceUnavailable, retryLater, "unavailable")
}

// timedOut is whether clamd gave up on a file for its scan time (MaxScanTime), which a later try
// may not hit, rather than for a limit the file itself exceeds.
func timedOut(sig string) bool {
	return strings.HasSuffix(sig, ".MaxScanTime")
}
