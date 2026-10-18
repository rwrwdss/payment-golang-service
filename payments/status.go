package payments

// Status — этап, на котором находится платёж.
type Status string

const (
	StatusNew     Status = "new"
	StatusWaiting Status = "waiting"
	StatusSuccess Status = "success"
	StatusDenied  Status = "denied"
)
