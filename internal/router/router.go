package router

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"elevate/internal/config"
	"elevate/internal/database"
	"elevate/internal/handler"
	"elevate/internal/repository"
	"elevate/internal/service"
)

func New(
	ctx context.Context,
	cfg *config.Config,
	db *database.DB,
	redisClient *redis.Client,
) *gin.Engine {
	// Keep Gin quiet. We use our own startup and request logging.
	gin.SetMode(gin.ReleaseMode)

	leadRepo := repository.NewLeadRepo(db)
	campaignRepo := repository.NewCampaignRepo(db)
	callRepo := repository.NewCallRepo(db)
	callbackRepo := repository.NewCallbackRepo(db)
	conversationRepo := repository.NewConversationRepo(db)
	discoveryRepo := repository.NewDiscoveryRepo(db)
	classificationRepo := repository.NewClassificationRepo(db)
	actionRepo := repository.NewActionRepo(db)
	whatsappRepo := repository.NewWhatsappRepo(db)
	assetRepo := repository.NewAssetRepo(db)
	webhookRepo := repository.NewWebhookRepo(db)

	twilioClient := service.NewTwilioClient(
		cfg.Twilio.AccountSID,
		cfg.Twilio.AuthToken,
		cfg.Twilio.WhatsAppNumber,
		cfg.Twilio.WhatsAppStatusCallbackURL,
	)

	whatsappService := service.NewWhatsappService(
		twilioClient,
		whatsappRepo,
	)

	actionService := service.NewActionService(
		actionRepo,
	)

	callbackService := service.NewCallbackService(
		callbackRepo,
	)

	webhookService := service.NewWebhookService(
		webhookRepo,
	)

	callService := service.NewCallService(
		cfg,
		callRepo,
		leadRepo,
		campaignRepo,
		twilioClient,
		callbackRepo,
		actionService,
	)

	agentFunctionExecutor :=
		service.NewAgentFunctionExecutor(
			discoveryRepo,
			classificationRepo,
			callRepo,
			actionService,
			callbackService,
		)

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(
			cfg.AWS.Region,
		),
	)
	if err != nil {
		panic(err)
	}

	s3Client := s3.NewFromConfig(
		awsCfg,
	)

	assetService := service.NewAssetService(
		cfg,
		assetRepo,
		s3Client,
	)

	actionExecutor := service.NewActionExecutor(
		cfg,
		whatsappService,
		callbackService,
		callRepo,
		leadRepo,
		campaignRepo,
		assetService,
		callbackRepo,
		discoveryRepo,
		conversationRepo,
		classificationRepo,
	)

	actionWorker := service.NewActionWorker(
		actionRepo,
		actionExecutor,
		250*time.Millisecond,
	)

	actionWorker.Start(ctx)

	callbackWorker := service.NewCallbackWorker(
		callbackRepo,
		callService,
		time.Second,
	)

	callbackWorker.Start(ctx)

	sessionManager :=
		service.NewVoiceSessionManager(
			cfg.Deepgram,
			conversationRepo,
			agentFunctionExecutor,
			twilioClient,
		)

	recordingService, err :=
		service.NewRecordingServiceWithBaseURL(
			ctx,
			cfg.AWS,
			cfg.Twilio.AccountSID,
			cfg.Twilio.AuthToken,
			cfg.App.APIBaseURL,
		)
	if err != nil {
		panic(err)
	}

	healthHandler := handler.NewHealthHandler(
		db,
		redisClient,
	)

	leadHandler := handler.NewLeadHandler(
		leadRepo,
	)

	campaignHandler := handler.NewCampaignHandler(
		campaignRepo,
	)

	callHandler := handler.NewCallHandler(
		callService,
		callRepo,
		discoveryRepo,
		classificationRepo,
		recordingService,
	)

	callbackHandler := handler.NewCallbackHandler(
		callbackRepo,
		callRepo,
	)

	assetHandler := handler.NewAssetHandler(
		assetService,
	)

	webhookHandler := handler.NewWebhookHandler(
		callService,
		whatsappService,
		webhookService,
		cfg.Twilio.AuthToken,
	)

	mediaStreamHandler :=
		handler.NewMediaStreamHandler(
			sessionManager,
			conversationRepo,
		)

	recordingHandler :=
		handler.NewRecordingHandler(
			recordingService,
			callRepo,
			cfg.Twilio.AuthToken,
		)

	r := gin.New()

	r.Use(
		gin.Recovery(),
	)

	r.Use(
		requestLogger(),
	)

	r.Use(
		corsMiddleware(
			cfg.App.FrontendURL,
		),
	)

	r.GET(
		"/health",
		healthHandler.Health,
	)

	api := r.Group(
		"/api/v1",
	)

	{
		leads := api.Group(
			"/leads",
		)

		{
			leads.GET(
				"",
				leadHandler.List,
			)

			leads.POST(
				"",
				leadHandler.Create,
			)

			leads.GET(
				"/:id",
				leadHandler.Get,
			)
		}

		campaigns := api.Group(
			"/campaigns",
		)

		{
			campaigns.GET(
				"",
				campaignHandler.List,
			)

			campaigns.POST(
				"",
				campaignHandler.Create,
			)

			campaigns.GET(
				"/:id",
				campaignHandler.Get,
			)

			campaigns.PUT(
				"/:id",
				campaignHandler.Update,
			)

			campaigns.PATCH(
				"/:id/assets",
				campaignHandler.UpdateAssets,
			)
		}

		assets := api.Group(
			"/assets",
		)

		{
			assets.GET(
				"",
				assetHandler.List,
			)

			assets.POST(
				"",
				assetHandler.Upload,
			)

			assets.GET(
				"/:id",
				assetHandler.Get,
			)

			assets.DELETE(
				"/:id",
				assetHandler.Delete,
			)
		}

		calls := api.Group(
			"/calls",
		)

		{
			calls.POST(
				"",
				callHandler.Trigger,
			)

			calls.GET(
				"",
				callHandler.List,
			)

			calls.GET(
				"/:id",
				callHandler.Get,
			)

			calls.GET(
				"/:id/transcript",
				callHandler.Transcript,
			)

			calls.GET(
				"/:id/actions",
				callHandler.Actions,
			)

			calls.GET(
				"/:id/discovery",
				callHandler.Discovery,
			)

			calls.GET(
				"/:id/classification",
				callHandler.Classification,
			)

			calls.GET(
				"/:id/recording",
				callHandler.Recording,
			)
		}

		callbacks := api.Group(
			"/callbacks",
		)

		{
			callbacks.GET(
				"",
				callbackHandler.List,
			)

			callbacks.POST(
				"",
				callbackHandler.Create,
			)
		}
	}

	webhooks := r.Group(
		"/webhooks",
	)

	{
		twilio := webhooks.Group(
			"/twilio",
		)

		{
			twilio.POST(
				"/voice",
				webhookHandler.TwilioVoice,
			)

			twilio.POST(
				"/status",
				webhookHandler.TwilioStatus,
			)

			twilio.POST(
				"/recording",
				recordingHandler.TwilioRecording,
			)

			twilio.POST(
				"/whatsapp",
				webhookHandler.TwilioWhatsapp,
			)

			twilio.POST(
				"/whatsapp/status",
				webhookHandler.TwilioWhatsappStatus,
			)

			twilio.GET(
				"/media-stream",
				mediaStreamHandler.TwilioMediaStream,
			)
		}
	}

	r.NoRoute(
		func(c *gin.Context) {
			c.JSON(
				http.StatusNotFound,
				gin.H{
					"error": "not found",
				},
			)
		},
	)

	return r
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		fmt.Printf(
			"[%s] %s %s -> %d (%s)\n",
			time.Now().Format(
				time.RFC3339,
			),
			c.Request.Method,
			path,
			c.Writer.Status(),
			time.Since(start),
		)
	}
}

func corsMiddleware(allowedOrigins string) gin.HandlerFunc {
	origins := make(map[string]bool)

	for _, o := range strings.Split(allowedOrigins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins[o] = true
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if origins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header(
			"Access-Control-Allow-Methods",
			"GET, POST, PUT, PATCH, DELETE, OPTIONS",
		)

		c.Header(
			"Access-Control-Allow-Headers",
			"Content-Type, Authorization",
		)

		c.Header(
			"Access-Control-Allow-Credentials",
			"true",
		)

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
