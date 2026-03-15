package dispatch

// LocalOnlyPolicy always routes locally.
type LocalOnlyPolicy struct{}

func (LocalOnlyPolicy) Route(_ RoutingContext) (RoutingResult, error) {
	return RoutingResult{Machine: ""}, nil
}
