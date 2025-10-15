package cleaners

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/go-azure-sdk/sdk/client"
	"github.com/hashicorp/go-azure-sdk/sdk/client/pollers"
	"github.com/hashicorp/go-azure-sdk/sdk/client/resourcemanager"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
)

var _ pollers.PollerType = &netappLROPoller{}

type netappLROPoller struct {
	client              LROClient
	azureAsyncOperation *url.URL
}

type LROClient interface {
	NewRequest(ctx context.Context, opts client.RequestOptions) (*http.Request, error)
	Execute(ctx context.Context, req *http.Request) (*http.Response, error)
}

func NewLROPoller(client LROClient, response *http.Response) (*netappLROPoller, error) {
	if response == nil {
		return nil, errors.New("no response provided")
	}
	bodyString := "[not able to read response body]"
	bodyBytes, err := io.ReadAll(response.Body)
	if err == nil {
		bodyString = string(bodyBytes)
	}
	// Restore the response body so it can be read again later if needed
	response.Body.Close()
	response.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	if response.StatusCode == http.StatusBadRequest {
		return nil, fmt.Errorf("invalid response status: %d, body: %s", response.StatusCode, bodyString)
	}
	if !slices.Contains([]int{http.StatusAccepted, http.StatusOK}, response.StatusCode) {
		return nil, fmt.Errorf("invalid response status: %d, body: %s", response.StatusCode, bodyString)
	}

	pollingUrl := response.Header.Get("Azure-AsyncOperation")
	if pollingUrl == "" {
		pollingUrl = response.Header.Get("Location")
	}

	if pollingUrl == "" {
		return nil, errors.New("no polling URL found in response (neither Azure-AsyncOperation nor Location headers were present)")
	}

	url, err := url.Parse(pollingUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid polling URL %q in response: %v", pollingUrl, err)
	}
	if !url.IsAbs() {
		return nil, fmt.Errorf("invalid polling URL %q in response: URL was not absolute", pollingUrl)
	}
	url.Query().Encode()
	return &netappLROPoller{
		client:              client,
		azureAsyncOperation: url,
	}, nil
}

type myOptions struct {
	azureAsyncOperation string
}

var _ client.Options = myOptions{}

func (p myOptions) ToHeaders() *client.Headers {
	return &client.Headers{}
}

func (p myOptions) ToOData() *odata.Query {
	return &odata.Query{}
}

func (p myOptions) ToQuery() *client.QueryParams {
	u, err := url.Parse(p.azureAsyncOperation)
	if err != nil {
		log.Printf("[ERROR] Unable to parse Azure-AsyncOperation URL: %v", err)
		return nil
	}
	q := client.QueryParams{}
	for k, v := range u.Query() {
		if len(v) > 0 {
			q.Append(k, v[0])
		}
	}
	return &q
}

func (p netappLROPoller) Poll(ctx context.Context) (*pollers.PollResult, error) {
	if p.azureAsyncOperation == nil {
		return nil, errors.New("internal error: cannot poll without a polling URL")
	}

	reqOpts := client.RequestOptions{
		ContentType: "application/json; charset=utf-8",
		ExpectedStatusCodes: []int{
			http.StatusOK,
			http.StatusAccepted,
			http.StatusConflict,
		},
		HttpMethod: http.MethodGet,
		Path:       p.azureAsyncOperation.Path,
		OptionsObject: myOptions{
			azureAsyncOperation: p.azureAsyncOperation.String(),
		},
	}

	req, err := p.client.NewRequest(ctx, reqOpts)
	if err != nil {
		return nil, fmt.Errorf("building request: %+v", err)
	}

	res, err := p.client.Execute(ctx, req)
	if err != nil {
		if res == nil || res.StatusCode >= 500 {
			return nil, fmt.Errorf("executing request: %+v", err)
		}
	}
	var respBody struct {
		Status          string  `json:"status"` // "InProgress",  "Succeeded"
		PercentComplete float32 `json:"percentComplete"`
		Error           struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(res.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("decoding response body: %+v", err)
	}
	if strings.HasPrefix(respBody.Error.Message, "Cannot delete resource while nested resources exist") {
		return &pollers.PollResult{
			Status:       pollers.PollingStatusInProgress,
			PollInterval: 10 * time.Second,
		}, nil
	}
	if respBody.Status == "Failed" {
		return nil, pollers.PollingFailedError{
			Message: respBody.Error.Message,
			HttpResponse: &client.Response{
				Response: res,
			},
		}
	}
	if slices.Contains([]string{"InProgress", "Deleting"}, respBody.Status) {
		return &pollers.PollResult{
			Status:       pollers.PollingStatusInProgress,
			PollInterval: 10 * time.Second,
		}, nil
	}
	if respBody.Status == "Succeeded" {
		return &pollers.PollResult{
			Status:       pollers.PollingStatusSucceeded,
			PollInterval: 10 * time.Second,
		}, nil
	}

	return nil, fmt.Errorf("unexpected status: %s", respBody.Status)
}

// lroClientAdapter adapts a *resourcemanager.Client to the LROClient interface expected by the poller.
type lroClientAdapter struct {
	inner *resourcemanager.Client
}

func (a *lroClientAdapter) NewRequest(ctx context.Context, opts client.RequestOptions) (*http.Request, error) {
	cReq, err := a.inner.NewRequest(ctx, opts)
	if err != nil {
		return nil, err
	}
	return cReq.Request, nil
}

func (a *lroClientAdapter) Execute(ctx context.Context, req *http.Request) (*http.Response, error) {
	cReq := &client.Request{Request: req, Client: a.inner}
	cReq.ValidStatusCodes = []int{http.StatusOK, http.StatusAccepted, http.StatusConflict}
	resp, err := a.inner.Execute(ctx, cReq)
	if err != nil {
		return resp.Response, err
	}

	return resp.Response, nil
}
