package routes

import "github.com/complyue/hbigo"

func NewServiceContext() hbi.HoContext {
	return &ServiceContext{
		HoContext: hbi.NewHoContext(),
	}
}

type ServiceContext struct {
	hbi.HoContext
}
