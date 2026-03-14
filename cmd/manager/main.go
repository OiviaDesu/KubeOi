/*
Copyright 2026 oiviadesu.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	geov1alpha1 "github.com/oiviadesu/oiviak3s-operator/api/v1alpha1"
	"github.com/oiviadesu/oiviak3s-operator/controllers"
	"github.com/oiviadesu/oiviak3s-operator/pkg/config"
	"github.com/oiviadesu/oiviak3s-operator/pkg/health"
	kubeletcheck "github.com/oiviadesu/oiviak3s-operator/pkg/health/kubelet"
	networkcheck "github.com/oiviadesu/oiviak3s-operator/pkg/health/network"
	resourcecheck "github.com/oiviadesu/oiviak3s-operator/pkg/health/resource"
	"github.com/oiviadesu/oiviak3s-operator/pkg/notification"
	"github.com/oiviadesu/oiviak3s-operator/pkg/notification/discord"
	"github.com/oiviadesu/oiviak3s-operator/pkg/notification/telegram"
	"github.com/oiviadesu/oiviak3s-operator/pkg/placement"
	"github.com/oiviadesu/oiviak3s-operator/pkg/placement/strategy"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(geov1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Load configuration from environment
	setupLog.Info("loading configuration from environment")
	loader := config.NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		setupLog.Error(err, "unable to load configuration")
		os.Exit(1)
	}

	setupLog.Info("configuration loaded",
		"healthCheckInterval", cfg.HealthCheckInterval,
		"placementStrategy", cfg.PlacementStrategy,
		"defaultRegion", cfg.DefaultRegionPreference)

	// Create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: cfg.MetricsBindAddress},
		HealthProbeBindAddress: cfg.HealthProbeBindAddress,
		LeaderElection:         cfg.LeaderElect,
		LeaderElectionID:       "oiviak3s-operator-leader",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Initialize components with dependency injection
	setupLog.Info("initializing components")

	// Create health provider with registered checkers
	healthProvider := setupHealthProvider(mgr.GetClient(), cfg, setupLog)

	// Create placement engine with registered strategies
	placementEngine := setupPlacementEngine(cfg, setupLog)

	// Create notification manager with registered notifiers
	notificationMgr := setupNotificationManager(cfg, setupLog)

	// Create controllers with injected dependencies
	setupLog.Info("setting up controllers")

	if err = controllers.NewNodeHealthStatusReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		healthProvider,
		ctrl.Log.WithName("controllers").WithName("NodeHealthStatus"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodeHealthStatus")
		os.Exit(1)
	}

	if err = controllers.NewRegionalWorkloadReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		placementEngine,
		cfg.SharedEndpointEnabled,
		cfg.SharedEndpointMode,
		cfg.SharedEndpointIP,
		cfg.SharedEndpointAutoFailback,
		ctrl.Log.WithName("controllers").WithName("RegionalWorkload"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RegionalWorkload")
		os.Exit(1)
	}

	if err = controllers.NewFailoverPolicyReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		notificationMgr,
		ctrl.Log.WithName("controllers").WithName("FailoverPolicy"),
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "FailoverPolicy")
		os.Exit(1)
	}

	// Setup health and readiness probes
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// setupHealthProvider creates and configures the health provider with all checkers
// Following OCP (Open-Closed Principle) - new checkers can be added without modifying this function's structure
func setupHealthProvider(client client.Client, cfg *config.Config, logger logr.Logger) health.Provider {
	logger.Info("setting up health provider")

	provider := health.NewRegistry(logger)

	// Register kubelet health checker
	kubeletChecker := kubeletcheck.NewChecker(logger)
	provider.RegisterChecker(kubeletChecker)
	logger.Info("registered health checker", "checker", "kubelet")

	// Register resource health checker
	resourceChecker := resourcecheck.NewChecker(logger, resourcecheck.Config{
		CPUThresholdPercent:  85.0,
		MemThresholdPercent:  85.0,
		DiskThresholdPercent: 85.0,
	})
	provider.RegisterChecker(resourceChecker)
	logger.Info("registered health checker", "checker", "resource")

	// Register network health checker
	networkChecker := networkcheck.NewChecker(logger, networkcheck.Config{
		ZerotierInterface: cfg.ZerotierInterface,
		PingTimeout:       cfg.HealthCheckTimeout,
	})
	provider.RegisterChecker(networkChecker)
	logger.Info("registered health checker", "checker", "network")

	return provider
}

// setupPlacementEngine creates and configures the placement engine with all strategies
// Following OCP (Open-Closed Principle) - new strategies can be added without modifying this function's structure
func setupPlacementEngine(cfg *config.Config, logger logr.Logger) placement.Engine {
	logger.Info("setting up placement engine")

	engine := placement.NewEngine(logger)

	// Register geographic placement strategy with weight 40
	geoStrategy := strategy.NewGeographic()
	engine.RegisterStrategy(geoStrategy, 40)
	logger.Info("registered placement strategy", "strategy", "geographic", "weight", 40)

	// Register resource-aware placement strategy with weight 35
	resourceStrategy := strategy.NewResourceAware()
	engine.RegisterStrategy(resourceStrategy, 35)
	logger.Info("registered placement strategy", "strategy", "resource-aware", "weight", 35)

	// Register tier-based placement strategy with weight 25
	tierStrategy := strategy.NewTier()
	engine.RegisterStrategy(tierStrategy, 25)
	logger.Info("registered placement strategy", "strategy", "tier-based", "weight", 25)

	return engine
}

// setupNotificationManager creates and configures the notification manager with all notifiers
// Following OCP (Open-Closed Principle) - new notifiers can be added without modifying this function's structure
func setupNotificationManager(cfg *config.Config, logger logr.Logger) notification.Manager {
	logger.Info("setting up notification manager")

	manager := notification.NewManager(logger)

	// Register Telegram notifier if configured
	if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
		telegramNotifier := telegram.NewNotifier(logger, telegram.Config{
			BotToken: cfg.TelegramBotToken,
			ChatID:   cfg.TelegramChatID,
			Enabled:  cfg.NotificationEnabled,
		})
		manager.RegisterNotifier(telegramNotifier)
		logger.Info("registered notifier", "notifier", "telegram")
	} else {
		logger.Info("telegram notifier not configured - skipping")
	}

	// Register Discord notifier if configured
	if cfg.DiscordWebhookURL != "" {
		discordNotifier := discord.NewNotifier(logger, discord.Config{
			WebhookURL: cfg.DiscordWebhookURL,
			Enabled:    cfg.NotificationEnabled,
		})
		manager.RegisterNotifier(discordNotifier)
		logger.Info("registered notifier", "notifier", "discord")
	} else {
		logger.Info("discord notifier not configured - skipping")
	}

	return manager
}
