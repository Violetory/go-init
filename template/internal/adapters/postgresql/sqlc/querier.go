// 模板占位接口，接入真实 SQL 后由 sqlc 生成结果替换。
package repo

import "context"

type Querier interface {
	Template(ctx context.Context) ([]Template, error)
}

var _ Querier = (*Queries)(nil)
