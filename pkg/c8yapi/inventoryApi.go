package c8yapi

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/tidwall/gjson"
)

type inventoryQueryOptions struct {
	query              string
	withChildren       bool
	withParents        bool
	withTotalElements  bool
	pageSize           int
	filterFn           func(item gjson.Result) bool
	keyExtractorFn     func(item gjson.Result) string
	valueTransformerFn func(item gjson.Result) gjson.Result
}

func NewInventoryQueryOptions(options ...func(*inventoryQueryOptions)) *inventoryQueryOptions {
	cfg := &inventoryQueryOptions{
		query:              "",
		withChildren:       false,
		withParents:        false,
		withTotalElements:  false,
		pageSize:           2000,
		filterFn:           func(item gjson.Result) bool { return true },
		keyExtractorFn:     func(item gjson.Result) string { return item.Get("id").String() },
		valueTransformerFn: func(item gjson.Result) gjson.Result { return item },
	}
	for _, o := range options {
		o(cfg)
	}
	return cfg
}

func WithValueTransformerFn(fn func(item gjson.Result) gjson.Result) func(*inventoryQueryOptions) {
	return func(cfg *inventoryQueryOptions) {
		cfg.valueTransformerFn = fn
	}
}

func WithKeyExtractorFn(fn func(item gjson.Result) string) func(*inventoryQueryOptions) {
	return func(cfg *inventoryQueryOptions) {
		cfg.keyExtractorFn = fn
	}
}

func WithFilterFn(fn func(item gjson.Result) bool) func(*inventoryQueryOptions) {
	return func(cfg *inventoryQueryOptions) {
		cfg.filterFn = fn
	}
}

func WithPageSize(pageSize int) func(*inventoryQueryOptions) {
	return func(cfg *inventoryQueryOptions) {
		cfg.pageSize = pageSize
	}
}

func WithQuery(query string) func(*inventoryQueryOptions) {
	return func(cfg *inventoryQueryOptions) {
		cfg.query = query
	}
}

func WithTotalElements() func(*inventoryQueryOptions) {
	return func(cfg *inventoryQueryOptions) {
		cfg.withTotalElements = true
	}
}

func WithChildren() func(*inventoryQueryOptions) {
	return func(cfg *inventoryQueryOptions) {
		cfg.withChildren = true
	}
}

func WithParents() func(*inventoryQueryOptions) {
	return func(cfg *inventoryQueryOptions) {
		cfg.withParents = true
	}
}

func QueryManagedObjects(client *c8y.Client, opts *inventoryQueryOptions) (map[string]gjson.Result, error) {
	initialPath := buildInventoryQueryString(*opts)

	res := make(map[string]gjson.Result)

	next := ""
	for {
		path := initialPath
		if len(next) > 0 {
			path = next
		}
		data := new(c8y.ManagedObjectCollection)
		resp, err := client.SendRequest(context.TODO(), c8y.RequestOptions{
			Method:       "GET",
			Path:         path,
			ResponseData: data,
		})
		if err != nil {
			return make(map[string]gjson.Result), err
		}
		if resp == nil {
			return make(map[string]gjson.Result),
				fmt.Errorf("server response for query '%s' is nil", path)
		}
		slog.Info("Logging Inventory API query",
			slog.Group("requestUrl",
				"host", resp.Response.Request.URL.Host,
				"path", resp.Response.Request.URL.Path,
				"query", resp.Response.Request.URL,
			),
			"method", resp.Response.Request.Method,
			"statusCode", resp.StatusCode(),
			"durationMs", resp.Duration().Milliseconds(),
			"ctElementsReturned", len(data.Items),
		)
		for _, i := range data.Items {
			if !opts.filterFn(i) {
				continue
			}
			res[opts.keyExtractorFn(i)] = opts.valueTransformerFn(i)
		}
		if len(data.Items) == 0 || data.Next == nil || *data.Statistics.CurrentPage == *data.Statistics.TotalPages {
			break
		}
		next = *data.Next
	}
	return res, nil
}

func buildInventoryQueryString(opts inventoryQueryOptions) string {
	var sb strings.Builder
	sb.WriteString("inventory/managedObjects")
	sb.WriteString(fmt.Sprintf("?pageSize=%d", opts.pageSize))
	sb.WriteString("&withTotalPages=true")
	sb.WriteString(fmt.Sprintf("&withTotalElements=%t", opts.withTotalElements))
	sb.WriteString(fmt.Sprintf("&withChildren=%t", opts.withChildren))
	sb.WriteString(fmt.Sprintf("&withParents=%t", opts.withParents))
	if len(opts.query) > 0 {
		sb.WriteString("&query=$filter=" + opts.query)
	}
	return sb.String()
}
