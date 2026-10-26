package claim

import "github.com/EnesGoker/guvenpolice/backend/internal/auth"

func externalRole(role auth.Role) bool {
	return role == auth.RoleCustomer || role == auth.RoleAgent
}

func publicStatus(status string) string {
	if status == "FRAUD_REVIEW" {
		return "UNDER_REVIEW"
	}
	return status
}

func visibleClaim(c Claim, role auth.Role) Claim {
	if externalRole(role) {
		c.Status = publicStatus(c.Status)
	}
	return c
}

func visibleDetail(d Detail, role auth.Role) Detail {
	if !externalRole(role) {
		return d
	}
	d.Claim = visibleClaim(d.Claim, role)
	d.Fraud, d.Tasks, d.AllowedTransitions = nil, nil, nil
	d.History = append([]History{}, d.History...)
	for i := range d.History {
		h := &d.History[i]
		fromReview := h.FromStatus != nil && *h.FromStatus == "FRAUD_REVIEW"
		if h.ToStatus != "REJECTED" && (h.ActorUserID == nil || h.ToStatus == "FRAUD_REVIEW" || fromReview) {
			h.Note = nil
		}
		if h.FromStatus != nil {
			from := publicStatus(*h.FromStatus)
			h.FromStatus = &from
		}
		h.ToStatus = publicStatus(h.ToStatus)
	}
	return d
}
