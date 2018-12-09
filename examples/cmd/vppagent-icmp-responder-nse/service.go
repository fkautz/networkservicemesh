// Copyright (c) 2018 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/binary"
	"github.com/ligato/vpp-agent/plugins/vpp/model/l2"
	"github.com/ligato/vpp-agent/plugins/vpp/model/rpc"
	"google.golang.org/grpc"
	"net"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/ligato/networkservicemesh/controlplane/pkg/apis/local/connection"
	"github.com/ligato/networkservicemesh/controlplane/pkg/apis/local/networkservice"
	"github.com/ligato/networkservicemesh/controlplane/pkg/local/monitor_connection_server"
	"github.com/sirupsen/logrus"
)

type vppagentNetworkService struct {
	sync.RWMutex
	networkService          string
	nextIP                  uint32
	monitorConnectionServer monitor_connection_server.MonitorConnectionServer
	vppAgentEndpoint        string
	baseDir                 string
	bridgeDomainName        string
}

func (ns *vppagentNetworkService) Request(ctx context.Context, request *networkservice.NetworkServiceRequest) (*connection.Connection, error) {
	logrus.Infof("Request for Network Service received %v", request)
	nseConnection, err := ns.CompleteConnection(request)
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	if err := ns.CreateVppInterface(ctx, nseConnection, ns.baseDir); err != nil {
		return nil, err
	}

	ns.monitorConnectionServer.UpdateConnection(nseConnection)
	logrus.Infof("Responding to NetworkService.Request(%v): %v", request, nseConnection)
	return nseConnection, nil
}

func (ns *vppagentNetworkService) Close(_ context.Context, conn *connection.Connection) (*empty.Empty, error) {
	// remove from connection
	ns.monitorConnectionServer.DeleteConnection(conn)
	return &empty.Empty{}, nil
}

func (ns *vppagentNetworkService) CreateBridgeDomain(vppAgentString string, bridgeDomainName string) error {
	conn, err := grpc.Dial(ns.vppAgentEndpoint, grpc.WithInsecure())
	if err != nil {
		logrus.Errorf("can't dial grpc server: %v", err)
		return err
	}
	defer conn.Close()
	client := rpc.NewDataChangeServiceClient(conn)

	bridgeDomain := &l2.BridgeDomains_BridgeDomain{
		Name:                bridgeDomainName,
		Flood:               true,
		UnknownUnicastFlood: true,
		Forward:             false,
		Learn:               true,
		ArpTermination:      false,
		MacAge:              0,
	}

	bridgeDomains := &l2.BridgeDomains{
		BridgeDomains: []*l2.BridgeDomains_BridgeDomain{bridgeDomain},
	}

	dataChange := &rpc.DataRequest{
		BridgeDomains: bridgeDomains.BridgeDomains,
	}

	logrus.Infof("Sending DataChange to vppagent: %v", dataChange)
	ctx := context.Background()
	if _, err := client.Put(ctx, dataChange); err != nil {
		logrus.Error(err)
		client.Del(ctx, dataChange)
		return err
	}
	return nil
}

func ip2int(ip net.IP) uint32 {
	if ip == nil {
		return 0
	}
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

func New(vppAgentEndpoint, baseDir, ip, bridgeDomainName string) networkservice.NetworkServiceServer {
	monitor := monitor_connection_server.NewMonitorConnectionServer()
	netIP := net.ParseIP(ip)
	service := vppagentNetworkService{
		networkService:          NetworkServiceName,
		nextIP:                  ip2int(netIP),
		monitorConnectionServer: monitor,
		vppAgentEndpoint:        vppAgentEndpoint,
		baseDir:                 baseDir,
		bridgeDomainName:        bridgeDomainName,
	}
	service.Reset()
	service.CreateBridgeDomain(vppAgentEndpoint, bridgeDomainName)
	return &service
}
