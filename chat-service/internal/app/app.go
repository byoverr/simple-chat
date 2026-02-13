package app

//type App struct {
//	GRPCServer *grpcapp.App
//	Storage    *postgres.Store
//}
//
//func New(
//	log zerolog.Logger,
//	cfg *config.Config,
//) *App {
//
//	// 1. Storage
//	storage, err := postgres.Connect(cfg.DB.PostgresURL, "") // schema="" for public or default
//	if err != nil {
//		panic(err)
//	}
//
//
//	// 3. Usecase
//	ucCfg := usecase.UsecaseConfig{
//		PasswordMinLen:     cfg.Auth.Pass.MinLen,
//		RefreshReuseDetect: cfg.Auth.Refresh.ReuseDetect,
//	}
//
//	authService := usecase.NewService(log, storage, hasher, jwtSigner, refreshTokens, ucCfg)
//
//	// 4. gRPC
//	grpcApp := grpcapp.New(log, authService, jwtSigner, cfg.GRPC.Addr)
//
//	return &App{
//		GRPCServer: grpcApp,
//		Storage:    storage,
//	}
//}
