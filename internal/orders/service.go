package orders

import (
	"context"
	"errors"
	"fmt"
	repo "github.com/Violetory/e-com/internal/adapters/postgresql/sqlc"
	"github.com/jackc/pgx/v5"
)

type Service interface {
	PlaceOrder(
		ctx context.Context,
		params createOrderParams,
	) (repo.Order, error)
}

// 错误提示定义
var (
	ErrProductNotFound = errors.New("商品不存在")
	ErrProductNoStock  = errors.New("商品库存不足")
)

type service struct {
	repo *repo.Queries
	db   *pgx.Conn
}

func NewService(repo *repo.Queries, db *pgx.Conn) Service {
	return &service{repo: repo, db: db}
}

func (s *service) PlaceOrder(ctx context.Context, params createOrderParams) (repo.Order, error) {
	// 校验入参
	if params.CustomerID == 0 {
		return repo.Order{}, fmt.Errorf("customerId 不能为空")
	}
	if len(params.Items) == 0 {
		return repo.Order{}, fmt.Errorf("订单项至少包含一个商品")
	}
	for _, item := range params.Items {
		if item.Quantity <= 0 {
			return repo.Order{}, fmt.Errorf("商品购买数量必须大于 0")
		}
	}

	// 开启事务
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.Order{}, err
	}
	defer tx.Rollback(ctx)

	qtx := s.repo.WithTx(tx)

	// 创建订单
	order, err := qtx.CreateOrder(ctx, params.CustomerID)
	if err != nil {
		return repo.Order{}, err
	}

	// 遍历商品判断是否存在或满足库存并创建订单明细
	for _, item := range params.Items {
		product, err := qtx.GetProductByID(ctx, item.ProductID)
		if errors.Is(err, pgx.ErrNoRows) {
			return repo.Order{}, ErrProductNotFound
		}
		if err != nil {
			return repo.Order{}, err
		}
		if product.Quantity < item.Quantity {
			return repo.Order{}, ErrProductNoStock
		}

		// 创建订单明细
		_, err = qtx.CreateOrderItem(ctx, repo.CreateOrderItemParams{
			OrderID:    order.ID,
			ProductID:  item.ProductID,
			Quantity:   item.Quantity,
			PriceCents: product.Price,
		})
		if err != nil {
			return repo.Order{}, err
		}

		// 更新商品库存，使用条件更新避免并发下单导致超卖
		rowsAffected, err := qtx.DecreaseProductStock(ctx, repo.DecreaseProductStockParams{
			ID:       item.ProductID,
			Quantity: item.Quantity,
		})
		if err != nil {
			return repo.Order{}, err
		}
		if rowsAffected == 0 {
			return repo.Order{}, ErrProductNoStock
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return repo.Order{}, err
	}

	return order, nil
}
