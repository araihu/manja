package domain

type Publication struct {
	ProjectID  string
	RevisionID string
	Public     bool
	Path       string
	Hostname   string
}

type Actor struct {
	Anonymous  bool
	UserID     string
	ProjectIDs []string
}

func (p Publication) VisibleTo(actor Actor) bool {
	if p.Public {
		return true
	}
	if actor.Anonymous {
		return false
	}
	for _, id := range actor.ProjectIDs {
		if id == p.ProjectID {
			return true
		}
	}
	return false
}
