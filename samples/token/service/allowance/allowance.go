package allowance

import (
	"errors"
	"fmt"

	"github.com/s7techlab/cckit/router"
	"github.com/s7techlab/cckit/state"

	"github.com/s7techlab/hyperledger-fabric-samples/samples/token/service/balance"
)

var (
	ErrOwnerOnly             = errors.New(`owner only`)
	ErrAllowanceInsufficient = errors.New(`allowance insufficient`)
)

type Service struct {
	balance *balance.Service
}

func NewService(balance *balance.Service) *Service {
	return &Service{
		balance: balance,
	}
}

func (s *Service) GetAllowance(ctx router.Context, id *AllowanceId) (*Allowance, error) {
	if err := router.ValidateRequest(id); err != nil {
		return nil, err
	}

	allowance, err := State(ctx).Get(id, &Allowance{})
	if err != nil {
		if errors.Is(err, state.ErrKeyNotFound) {
			return &Allowance{
				Owner:   id.Owner,
				Spender: id.Spender,
				Symbol:  id.Symbol,
				Group:   id.Group,
				Amount:  0,
			}, nil
		}
		return nil, fmt.Errorf(`get allowance: %w`, err)
	}

	return allowance.(*Allowance), nil
}

func (s *Service) Approve(ctx router.Context, req *ApproveRequest) (*Allowance, error) {
	if err := router.ValidateRequest(req); err != nil {
		return nil, err
	}

	invokerAddress, err := s.balance.Account.GetInvokerAddress(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf(`get invoker address: %w`, err)
	}

	if invokerAddress.Address != req.Owner {
		return nil, ErrOwnerOnly
	}

	allowance := &Allowance{
		Owner:   req.Owner,
		Spender: req.Spender,
		Symbol:  req.Symbol,
		Group:   req.Group,
		Amount:  req.Amount,
	}

	if err := State(ctx).Put(allowance); err != nil {
		return nil, fmt.Errorf(`set allowance: %w`, err)
	}

	if err = Event(ctx).Set(&Approved{
		Owner:   req.Owner,
		Spender: req.Spender,
		Amount:  req.Amount,
	}); err != nil {
		return nil, err
	}

	return allowance, nil
}

func (s *Service) TransferFrom(ctx router.Context, req *TransferFromRequest) (*TransferFromResponse, error) {
	if err := router.ValidateRequest(req); err != nil {
		return nil, err
	}

	spenderAddress, err := s.balance.Account.GetInvokerAddress(ctx, nil)
	if err != nil {
		return nil, err
	}

	allowance, err := s.GetAllowance(ctx, &AllowanceId{
		Owner:   req.Owner,
		Spender: spenderAddress.Address,
		Symbol:  req.Symbol,
		Group:   req.Group,
	})
	if err != nil {
		return nil, err
	}

	if allowance.Amount < req.Amount {
		return nil, fmt.Errorf(`request trasfer amount=%d, allowance=%d: %w`,
			req.Amount, allowance.Amount, ErrAllowanceInsufficient)
	}

	allowance.Amount -= req.Amount
	// sub from allowance
	if err := State(ctx).Put(allowance); err != nil {
		return nil, fmt.Errorf(`update allowance: %w`, err)
	}

	if err = s.balance.Store.Transfer(ctx, &balance.TransferOperation{
		Sender:    req.Owner,
		Recipient: req.Recipient,
		Symbol:    req.Symbol,
		Group:     req.Group,
		Amount:    req.Amount,
	}); err != nil {
		return nil, err
	}

	if err = Event(ctx).Set(&TransferredFrom{
		Owner:     req.Owner,
		Spender:   spenderAddress.Address,
		Recipient: req.Recipient,
		Amount:    req.Amount,
	}); err != nil {
		return nil, err
	}

	return &TransferFromResponse{
		Owner:     req.Owner,
		Recipient: req.Recipient,
		Amount:    req.Amount,
	}, nil
}