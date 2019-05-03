package main

import (
	"context"

	"github.com/HotCodeGroup/warscript-utils/models"
	"google.golang.org/grpc"
)

type LocalGameClient struct{}

func (c *LocalGameClient) GetGameBySlug(ctx context.Context,
	in *models.GameSlug, opts ...grpc.CallOption) (*models.InfoGame, error) {
	return &models.InfoGame{
		Slug: "pong",
	}, nil
}
