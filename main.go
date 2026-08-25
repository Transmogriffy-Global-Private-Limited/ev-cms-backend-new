package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/db"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/config"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/cpo"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/customerauth"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/halops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/integrations"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/liveops"
	cmsmail "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/mail"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/operationalrealtime"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/platformops"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/routes"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/security"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/subscriptions"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/superadmin"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/support"
	"github.com/google/uuid"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	gormDB, sqlDB, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	if err := db.SeedSuperadmin(ctx, gormDB, cfg.Superadmin); err != nil {
		return fmt.Errorf("seed initial superadmin: %w", err)
	}

	tokenManager, err := security.NewTokenManager(
		cfg.Auth.Issuer,
		cfg.Auth.Audience,
		cfg.Auth.AccessTTL,
		cfg.Auth.SigningKey,
		cfg.Auth.EncryptionKey,
	)
	if err != nil {
		return err
	}
	mailSecretBox, err := security.NewSecretBox(
		cfg.Auth.MailOutbox.KeyID,
		cfg.Auth.MailOutbox.Key,
	)
	if err != nil {
		return fmt.Errorf("initialize mail payload encryption: %w", err)
	}
	credentialSecretBox, err := security.NewSecretBox(
		cfg.Credentials.KeyID,
		cfg.Credentials.Key,
	)
	if err != nil {
		return fmt.Errorf("initialize credential encryption: %w", err)
	}
	outbox := cmsmail.NewOutbox(mailSecretBox)
	authService, err := auth.NewService(
		gormDB,
		cfg.Auth,
		cfg.Mail.Enabled,
		outbox,
		tokenManager,
	)
	if err != nil {
		return err
	}
	processInstanceKey := uuid.NewString()
	platformMaintenanceInstanceKey := processInstanceKey + ":platform-maintenance"
	halReconcilerInstanceKey := processInstanceKey + ":hal-reconciler"
	operationalRetentionInstanceKey := processInstanceKey + ":operational-retention"
	mailOutboxInstanceKey := processInstanceKey + ":mail-outbox"
	subscriptionLifecycleInstanceKey := processInstanceKey + ":subscription-lifecycle"
	platformService := platformops.NewService(gormDB, cfg.Platform)
	superadminService := superadmin.NewService(gormDB, platformService, outbox, cfg.Mail.Enabled)
	subscriptionService := subscriptions.NewService(gormDB, platformService)
	supportService := support.NewService(gormDB)
	halOperations := halops.New(gormDB, cfg.HAL)
	platformService.WithHALFactRequeuer(halOperations)
	liveOperations := liveops.New(gormDB, cfg.HAL)
	operationalEvents := operationalrealtime.New(gormDB, cfg.Platform)
	platformService.WithExpectedWorkers([]platformops.WorkerSpec{
		{Name: "platform-maintenance", InstanceKey: platformMaintenanceInstanceKey, Required: true, Enabled: true},
		{Name: "hal-reconciler", InstanceKey: halReconcilerInstanceKey, Required: true, Enabled: halOperations.Available()},
		{Name: "operational-retention", InstanceKey: operationalRetentionInstanceKey, Required: false, Enabled: true},
		{Name: "mail-outbox", InstanceKey: mailOutboxInstanceKey, Required: true, Enabled: cfg.Mail.Enabled},
		{Name: "subscription-lifecycle", InstanceKey: subscriptionLifecycleInstanceKey, Required: true, Enabled: true},
	})
	halOperations.WithWorkerObserver(platformService, "hal-reconciler", halReconcilerInstanceKey)
	operationalEvents.WithWorkerObserver(platformService, "operational-retention", operationalRetentionInstanceKey)
	cpoService := cpo.NewService(gormDB, outbox, cfg.Mail.Enabled, cfg.ChargerConnectionURL).
		WithPlatformEvents(platformService).
		WithOperationalCapabilities(halOperations, liveOperations).
		WithOperationalEvents(operationalEvents)
	customerAuthService, err := customerauth.NewService(
		gormDB, cfg.Auth, cfg.Mail.Enabled, outbox, tokenManager,
	)
	if err != nil {
		return err
	}
	customerAuthService.WithHALOperations(halOperations, liveOperations, cfg.HAL).WithOperationalEvents(operationalEvents)
	integrationService := integrations.NewService(gormDB, credentialSecretBox)
	customerAuthService.WithRazorpayCredentialResolver(func(
		ctx context.Context,
		cpoID uuid.UUID,
	) (customerauth.RazorpayCredentials, error) {
		credentials, err := integrationService.ResolveRazorpay(ctx, cpoID)
		if err != nil {
			return customerauth.RazorpayCredentials{}, err
		}
		return customerauth.RazorpayCredentials{
			KeyID: credentials.KeyID, KeySecret: credentials.KeySecret,
			WebhookSecret: credentials.WebhookSecret,
		}, nil
	})
	go platformService.RunMaintenance(ctx, platformMaintenanceInstanceKey)
	if halOperations.Available() {
		go halOperations.RunReconciler(ctx, time.Minute)
	}
	go operationalEvents.RunRetention(ctx, cfg.Platform.MaintenanceEvery)
	go subscriptionService.RunLifecycle(ctx, cfg.Platform.MaintenanceEvery, platformService, subscriptionLifecycleInstanceKey)

	if cfg.Mail.Enabled {
		sender, err := cmsmail.NewSMTPSender(cfg.Mail)
		if err != nil {
			return err
		}
		worker := cmsmail.NewWorker(
			gormDB,
			mailSecretBox,
			sender,
			cfg.Mail.WorkerPoll,
			cfg.Mail.SendTimeout,
		).WithObserver(
			platformService,
			"mail-outbox",
			mailOutboxInstanceKey,
		)
		go worker.Run(ctx)
	}

	server := &http.Server{
		Addr: cfg.HTTPAddress,
		Handler: routes.New(
			sqlDB,
			authService,
			customerAuthService,
			cpoService,
			integrationService,
			platformService,
			subscriptionService,
			supportService,
			cfg.CORSAllowAll,
			cfg.APIDocsEnabled,
			os.Stdout,
			cfg.LogLevel == config.LogLevelDebug,
			superadminService,
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("EV CMS listening on http://%s", cfg.HTTPAddress)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("shut down HTTP server: %w", err)
	}

	return nil
}
