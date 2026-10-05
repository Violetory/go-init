package repo

import "context"

// Template 保留分层调用示例，不执行 SQL。真实生成后移除或调整本文件。
func (q *Queries) Template(ctx context.Context) ([]Template, error) {
	return []Template{}, nil
}
