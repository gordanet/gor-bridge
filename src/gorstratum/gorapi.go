package gorstratum

import (
	"context"
	"fmt"
	"time"

	"github.com/gordanet/gord/app/appmessage"
	"github.com/gordanet/gord/infrastructure/network/rpcclient"
	"github.com/gordanet/gord/infrastructure/network/rpcclient"
	"github.com/pkg/errors"
	"go.uber.org/zap"
    "db"
)

type gorApi struct {
	address       string
	blockWaitTime time.Duration
	logger        *zap.SugaredLogger
	gord        *rpcclient.RPCClient
	connected     bool
}
func NewgorAPI(address string, blockWaitTime time.Duration, logger *zap.SugaredLogger) (*gorApi, error) {
	client, err := rpcclient.NewRPCClient(address)
	if err != nil {
		return nil, err
	}
	return &gorApi{
		address:       address,
		blockWaitTime: blockWaitTime,
		logger:        logger.With(zap.String("component", "gorapi:"+address)),
		gord:        client,
		connected:     true,
	}, nil
}
func (ks *gorApi) Start(ctx context.Context, blockCb func()) {
	ks.waitForSync(true)
	go ks.startBlockTemplateListener(ctx, blockCb)
	go ks.startStatsThread(ctx)
}

func (ks *gorApi) startStatsThread(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ctx.Done():
			ks.logger.Warn("context cancelled, stopping stats thread")
			return
		case <-ticker.C:
			dagResponse, err := ks.gord.GetBlockDAGInfo()
			if err != nil {
				ks.logger.Warn("failed to get network hashrate from gor, prom stats will be out of date", zap.Error(err))
				continue
			}
			response, err := ks.gord.EstimateNetworkHashesPerSecond(dagResponse.TipHashes[0], 1000)
			if err != nil {
				ks.logger.Warn("failed to get network hashrate from gor, prom stats will be out of date", zap.Error(err))
				continue
			}
			RecordNetworkStats(response.NetworkHashesPerSecond, dagResponse.BlockCount, dagResponse.Difficulty)
        rows,err := db.DB.Query("select id from poolstats order by id desc limit 1")
        checkError(err)
        defer rows.Close()
        var idd int
for rows.Next() {
	err=rows.Scan(&idd)
	checkError(err)
}
	p1:=dagResponse.BlockCount
	p2:=dagResponse.Difficulty
	p3:=response.NetworkHashesPerSecond
        _,err=db.DB.Exec("update poolstats set blockheight=$1, networkdifficulty=$2, networkhashrate=$3 where id>=$4", p1, p2, p3, idd)
        checkError(err)
		}
	}
}

func (ks *gorApi) reconnect() error {
	if ks.gord != nil {
		return ks.gord.Reconnect()
	}

	client, err := rpcclient.NewRPCClient(ks.address)
	if err != nil {
		return err
	}
	ks.gord = client
	return nil
}

func (s *gorApi) waitForSync(verbose bool) error {
	if verbose {
		s.logger.Info("checking gord sync state")
	}
	for {
		clientInfo, err := s.gord.GetInfo()
		if err != nil {
			return errors.Wrapf(err, "error fetching server info from gord @ %s", s.address)
		}
		if clientInfo.IsSynced {
			break
		}
		s.logger.Warn("gor is not synced, waiting for sync before starting bridge")
		time.Sleep(5 * time.Second)
		break
	}
	if verbose {
		s.logger.Info("gord synced, starting server")
	}
	return nil
}

func (s *gorApi) startBlockTemplateListener(ctx context.Context, blockReadyCb func()) {
	blockReadyChan := make(chan bool)
	err := s.gord.RegisterForNewBlockTemplateNotifications(func(_ *appmessage.NewBlockTemplateNotificationMessage) {
		blockReadyChan <- true
	})
	if err != nil {
		s.logger.Error("fatal: failed to register for block notifications from gor")
	}

	ticker := time.NewTicker(s.blockWaitTime)
	for {
		if err := s.waitForSync(false); err != nil {
			s.logger.Error("error checking gord sync state, attempting reconnect: ", err)
			if err := s.reconnect(); err != nil {
				s.logger.Error("error reconnecting to gord, waiting before retry: ", err)
				time.Sleep(5 * time.Second)
			}
		}
		select {
		case <-ctx.Done():
			s.logger.Warn("context cancelled, stopping block update listener")
			return
		case <-blockReadyChan:
			blockReadyCb()
			ticker.Reset(s.blockWaitTime)
		case <-ticker.C: // timeout, manually check for new blocks
			blockReadyCb()
		}
	}
}

func (ks *gorApi) GetBlockTemplate(
	client *gostratum.StratumContext) (*appmessage.GetBlockTemplateResponseMessage, error) {
	template, err := ks.gord.GetBlockTemplate(client.WalletAddr,
		fmt.Sprintf(`'%s' via onemorebsmith/gor-stratum-bridge_%s`, client.RemoteApp, version))
	if err != nil {
		return nil, errors.Wrap(err, "failed fetching new block template from gor")
	}
	return template, nil
}
