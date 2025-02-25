// Code generated by protoc-gen-micro. DO NOT EDIT.
// source: proto/alerts.proto

package alerts

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

import (
	context "context"
	api "github.com/micro/micro/v3/service/api"
	client "github.com/micro/micro/v3/service/client"
	server "github.com/micro/micro/v3/service/server"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

// Reference imports to suppress errors if they are not otherwise used.
var _ api.Endpoint
var _ context.Context
var _ client.Option
var _ server.Option

// Api Endpoints for Alerts service

func NewAlertsEndpoints() []*api.Endpoint {
	return []*api.Endpoint{}
}

// Client API for Alerts service

type AlertsService interface {
	// ReportEvent does event ingestions.
	ReportEvent(ctx context.Context, in *ReportEventRequest, opts ...client.CallOption) (*ReportEventResponse, error)
}

type alertsService struct {
	c    client.Client
	name string
}

func NewAlertsService(name string, c client.Client) AlertsService {
	return &alertsService{
		c:    c,
		name: name,
	}
}

func (c *alertsService) ReportEvent(ctx context.Context, in *ReportEventRequest, opts ...client.CallOption) (*ReportEventResponse, error) {
	req := c.c.NewRequest(c.name, "Alerts.ReportEvent", in)
	out := new(ReportEventResponse)
	err := c.c.Call(ctx, req, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for Alerts service

type AlertsHandler interface {
	// ReportEvent does event ingestions.
	ReportEvent(context.Context, *ReportEventRequest, *ReportEventResponse) error
}

func RegisterAlertsHandler(s server.Server, hdlr AlertsHandler, opts ...server.HandlerOption) error {
	type alerts interface {
		ReportEvent(ctx context.Context, in *ReportEventRequest, out *ReportEventResponse) error
	}
	type Alerts struct {
		alerts
	}
	h := &alertsHandler{hdlr}
	return s.Handle(s.NewHandler(&Alerts{h}, opts...))
}

type alertsHandler struct {
	AlertsHandler
}

func (h *alertsHandler) ReportEvent(ctx context.Context, in *ReportEventRequest, out *ReportEventResponse) error {
	return h.AlertsHandler.ReportEvent(ctx, in, out)
}
