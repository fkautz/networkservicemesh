package main

import (
	"context"
	"github.com/ligato/vpp-agent/plugins/vpp/model/l2"
	"time"

	"github.com/ligato/networkservicemesh/controlplane/pkg/apis/local/connection"
	"github.com/ligato/networkservicemesh/dataplane/vppagent/pkg/converter"
	"github.com/ligato/networkservicemesh/pkg/tools"
	"github.com/ligato/vpp-agent/plugins/vpp/model/rpc"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

func (ns *vppagentNetworkService) CreateVppInterface(ctx context.Context, nseConnection *connection.Connection, baseDir string) error {
	conn, err := grpc.Dial(ns.vppAgentEndpoint, grpc.WithInsecure())
	if err != nil {
		logrus.Errorf("can't dial grpc server: %v", err)
		return err
	}
	defer conn.Close()
	client := rpc.NewDataChangeServiceClient(conn)

	conversionParameters := &converter.ConnectionConversionParameters{
		Name:      "DST-" + nseConnection.GetId(),
		Terminate: true,
		Side:      converter.DESTINATION,
		BaseDir:   baseDir,
	}

	ns.Lock()
	defer ns.Unlock()
	dataChange, err := converter.NewMemifInterfaceConverter(nseConnection, conversionParameters).ToDataRequest(ns.state)
	interfaceConfig := dataChange.Interfaces[len(dataChange.Interfaces)-1]
	dataChange.BridgeDomains[0].Interfaces = []*l2.BridgeDomains_BridgeDomain_Interfaces{{
		Name:                    interfaceConfig.Name,
		BridgedVirtualInterface: false,
		SplitHorizonGroup:       1,
	}}

	if err != nil {
		logrus.Error(err)
		return err
	}
	logrus.Infof("Sending DataChange to vppagent: %v", dataChange)
	if _, err := client.Put(ctx, dataChange); err != nil {
		logrus.Error(err)
		client.Del(ctx, dataChange)
		return err
	}
	return nil
}

func (ns *vppagentNetworkService) Reset() error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	tools.WaitForPortAvailable(ctx, "tcp", ns.vppAgentEndpoint, 100*time.Millisecond)
	conn, err := grpc.Dial(ns.vppAgentEndpoint, grpc.WithInsecure())
	if err != nil {
		logrus.Errorf("can't dial grpc server: %v", err)
		return err
	}
	defer conn.Close()
	client := rpc.NewDataResyncServiceClient(conn)
	logrus.Infof("Resetting vppagent...")
	_, err = client.Resync(context.Background(), &rpc.DataRequest{})
	if err != nil {
		logrus.Errorf("failed to reset vppagent: %s", err)
	}
	logrus.Infof("Finished resetting vppagent...")
	return nil
}

func (ns *vppagentNetworkService) CreateBridgeDomain(ctx context.Context, bridgeDomainName string) error {
	conn, err := grpc.Dial(ns.vppAgentEndpoint, grpc.WithInsecure())
	if err != nil {
		logrus.Errorf("can't dial grpc server: %v", err)
		return err
	}
	defer conn.Close()
	client := rpc.NewDataChangeServiceClient(conn)

	conversionParameters := &converter.BridgeDomainConversionParameters{
		Name: bridgeDomainName,
	}

	dataChange, err := converter.NewBridgeDomainConverter(conversionParameters).ToDataRequest(ns.state)
	if err != nil {
		logrus.Error(err)
		return err
	}

	logrus.Infof("Sending DataChange to vppagent: %v", dataChange)
	if _, err := client.Put(ctx, dataChange); err != nil {
		logrus.Error(err)
		client.Del(ctx, dataChange)
		return err
	}

	return nil
}
