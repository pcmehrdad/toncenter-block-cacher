package api

import (
	"context"
	"fmt"

	"github.com/xssnick/tonutils-go/ton"
)

type TonCenter struct {
	client *ton.APIClient
}

func NewTonCenter(client *ton.APIClient) *TonCenter {
	return &TonCenter{client: client}
}

func (tc *TonCenter) GetCurrentBlockNumber() (int, error) {
	info, err := tc.client.GetMasterchainInfo(context.Background())
	if err != nil {
		return 0, fmt.Errorf("get masterchain info: %w", err)
	}
	return int(info.SeqNo), nil
}

func (tc *TonCenter) IsBlockValid(blockNum int) (bool, error) {
	_, err := tc.client.LookupBlock(context.Background(), 0, -1, uint32(blockNum))
	return err == nil, nil
}
