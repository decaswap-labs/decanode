package monero

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type DaemonRPC struct {
	url    string
	client *http.Client
}

func NewDaemonRPC(url string) *DaemonRPC {
	return &DaemonRPC{
		url: url,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type GetBlockCountResult struct {
	Count  int64  `json:"count"`
	Status string `json:"status"`
}

type BlockHeader struct {
	Height    int64  `json:"height"`
	Hash      string `json:"hash"`
	Timestamp int64  `json:"timestamp"`
	NumTxes   int    `json:"num_txes"`
}

type GetBlockResult struct {
	BlockHeader BlockHeader `json:"block_header"`
	TxHashes    []string    `json:"tx_hashes"`
	JSON        string      `json:"json"`
	Status      string      `json:"status"`
}

type TransferDetail struct {
	Amount      uint64 `json:"amount"`
	Address     string `json:"address"`
	TxHash      string `json:"tx_hash"`
	BlockHeight int64  `json:"block_height"`
	PaymentID   string `json:"payment_id"`
}

type SubmitTxResult struct {
	Status string `json:"status"`
}

func (d *DaemonRPC) call(method string, params interface{}) (json.RawMessage, error) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      "0",
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rpc request: %w", err)
	}

	resp, err := d.client.Post(d.url+"/json_rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("rpc request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc request returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp jsonRPCResponse
	err = json.Unmarshal(respBody, &rpcResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal rpc response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func (d *DaemonRPC) GetHeight() (int64, error) {
	result, err := d.call("get_block_count", nil)
	if err != nil {
		return 0, err
	}

	var blockCount GetBlockCountResult
	err = json.Unmarshal(result, &blockCount)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal block count: %w", err)
	}

	return blockCount.Count, nil
}

func (d *DaemonRPC) GetBlock(height int64) (*GetBlockResult, error) {
	params := map[string]int64{"height": height}
	result, err := d.call("get_block", params)
	if err != nil {
		return nil, err
	}

	var block GetBlockResult
	err = json.Unmarshal(result, &block)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &block, nil
}

func (d *DaemonRPC) SubmitTransaction(txHex string) (string, error) {
	reqBody := map[string]string{"tx_as_hex": txHex}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal submit request: %w", err)
	}

	resp, err := d.client.Post(d.url+"/sendrawtransaction", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("submit transaction request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read submit response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("submit transaction returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result SubmitTxResult
	err = json.Unmarshal(respBody, &result)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal submit response: %w", err)
	}

	if result.Status != "OK" {
		return "", fmt.Errorf("submit transaction failed with status: %s", result.Status)
	}

	return result.Status, nil
}
