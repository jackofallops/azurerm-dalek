package expressroutecircuitauthorizations

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/go-azure-sdk/sdk/client"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
)

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See NOTICE.txt in the project root for license information.

type ListOperationResponse struct {
	HttpResponse *http.Response
	OData        *odata.OData
	Model        *[]ExpressRouteCircuitAuthorization
}

type ListCompleteResult struct {
	Items []ExpressRouteCircuitAuthorization
}

// List ...
func (c ExpressRouteCircuitAuthorizationsClient) List(ctx context.Context, id ExpressRouteCircuitId) (result ListOperationResponse, err error) {
	opts := client.RequestOptions{
		ContentType: "application/json; charset=utf-8",
		ExpectedStatusCodes: []int{
			http.StatusOK,
		},
		HttpMethod: http.MethodGet,
		Path:       fmt.Sprintf("%s/authorizations", id.ID()),
	}

	req, err := c.Client.NewRequest(ctx, opts)
	if err != nil {
		return
	}

	var resp *client.Response
	resp, err = req.ExecutePaged(ctx)
	if resp != nil {
		result.OData = resp.OData
		result.HttpResponse = resp.Response
	}
	if err != nil {
		return
	}

	var values struct {
		Values *[]ExpressRouteCircuitAuthorization `json:"value"`
	}
	if err = resp.Unmarshal(&values); err != nil {
		return
	}

	result.Model = values.Values

	return
}

// ListComplete retrieves all the results into a single object
func (c ExpressRouteCircuitAuthorizationsClient) ListComplete(ctx context.Context, id ExpressRouteCircuitId) (ListCompleteResult, error) {
	return c.ListCompleteMatchingPredicate(ctx, id, ExpressRouteCircuitAuthorizationOperationPredicate{})
}

// ListCompleteMatchingPredicate retrieves all the results and then applies the predicate
func (c ExpressRouteCircuitAuthorizationsClient) ListCompleteMatchingPredicate(ctx context.Context, id ExpressRouteCircuitId, predicate ExpressRouteCircuitAuthorizationOperationPredicate) (result ListCompleteResult, err error) {
	items := make([]ExpressRouteCircuitAuthorization, 0)

	resp, err := c.List(ctx, id)
	if err != nil {
		err = fmt.Errorf("loading results: %+v", err)
		return
	}
	if resp.Model != nil {
		for _, v := range *resp.Model {
			if predicate.Matches(v) {
				items = append(items, v)
			}
		}
	}

	result = ListCompleteResult{
		Items: items,
	}
	return
}
