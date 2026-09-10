package app

import (
	"fmt"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"net/http"
	"time"

	httpHandler "github.com/hengshu-credit/yaoguang-marketing/internal/http"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
)

func (a *App) initConfigurableMarketingRepositories() error {
	customers, ok := a.customerRepo.(*repository.CustomerPostgresRepository)
	if !ok {
		return fmt.Errorf("configurable marketing requires the customer identity repository")
	}
	a.businessDefinitionRepo = repository.NewBusinessConfigRepository(a.workspaceRepo)
	a.businessObjectRepo = repository.NewBusinessObjectRepository(a.workspaceRepo)
	a.trackedEventRepo = repository.NewTrackedEventRepository(a.workspaceRepo, customers)
	a.executionSnapshotRepo = repository.NewExecutionSnapshotRepository(a.workspaceRepo)
	a.subjectStateRepo = repository.NewSubjectStateRepository(a.workspaceRepo)
	a.funnelRepo = repository.NewFunnelRepository(a.workspaceRepo)
	return nil
}

func (a *App) initConfigurableMarketingServices() error {
	a.businessConfigService = service.NewBusinessConfigService(a.businessDefinitionRepo, a.authService)
	a.businessConfigService.SetCustomerRepository(a.customerRepo)
	a.businessObjectService = service.NewBusinessObjectService(a.businessObjectRepo, a.businessDefinitionRepo, a.authService)
	a.trackedEventService = service.NewTrackedEventService(a.trackedEventRepo, a.businessDefinitionRepo, a.authService, a.workspaceRepo)
	a.browserEventService = service.NewBrowserEventService(a.trackedEventRepo, a.businessDefinitionRepo, a.authService, a.workspaceRepo, a.customerRepo)
	a.subjectStateService = service.NewSubjectStateService(a.subjectStateRepo, a.funnelRepo, a.businessDefinitionRepo, a.authService)
	a.subjectStateService.SetCustomerRepository(a.customerRepo)
	a.subjectStateMaintenanceWorker = service.NewSubjectStateMaintenanceWorker(repository.NewWorkspaceCursorRepository(a.workspaceRepo), a.subjectStateService, a.logger)
	a.funnelService = service.NewFunnelService(a.funnelRepo, a.businessDefinitionRepo, a.authService)
	runtimeRepository := repository.NewRealtimeRepository(a.workspaceRepo)
	ruleWorker, err := service.NewRuleWorker(runtimeRepository, config.RealtimeModePrimary, time.Minute)
	if err != nil {
		return err
	}
	ruleWorker.SetStateProjector(a.subjectStateService)
	ruleWorker.SetConfiguredOnly(true)
	a.configuredEventWorker = service.NewConfiguredEventWorker(repository.NewWorkspaceCursorRepository(a.workspaceRepo), runtimeRepository, ruleWorker, a.logger)
	a.executionSnapshotService, err = service.NewExecutionSnapshotService(a.executionSnapshotRepo, a.authService)
	if err != nil {
		return fmt.Errorf("initialize execution snapshots: %w", err)
	}
	a.executionSnapshotService.SetTemplateRepository(a.templateRepo)
	a.executionSnapshotService.SetWorkspaceRepository(a.workspaceRepo)
	archiveBacklog, ok := a.executionSnapshotRepo.(service.AutomationArchiveBacklog)
	if !ok {
		return fmt.Errorf("execution repository does not support material backfill")
	}
	a.executionMaterialBackfillWorker = service.NewExecutionMaterialBackfillWorker(repository.NewWorkspaceCursorRepository(a.workspaceRepo), archiveBacklog, a.executionSnapshotService, a.logger)
	a.templateService.SetExecutionSnapshotService(a.executionSnapshotService)
	a.campaignSnapshotService.SetExecutionSnapshotService(a.executionSnapshotService)
	a.campaignSnapshotService.SetBusinessDefinitions(a.businessDefinitionRepo)
	return nil
}

func (a *App) registerConfigurableMarketingHandlers(mux *http.ServeMux, secret func() ([]byte, error)) {
	httpHandler.NewBusinessConfigHandler(a.businessConfigService, secret).RegisterRoutes(mux)
	httpHandler.NewBusinessObjectHandler(a.businessObjectService, secret).RegisterRoutes(mux)
	httpHandler.NewTrackedEventHandler(a.trackedEventService, secret).RegisterRoutes(mux)
	httpHandler.NewBrowserEventHandler(a.browserEventService, secret).RegisterRoutes(mux)
	httpHandler.NewSubjectStateHandler(a.subjectStateService, secret).RegisterRoutes(mux)
	httpHandler.NewFunnelHandler(a.funnelService, secret).RegisterRoutes(mux)
	httpHandler.NewExecutionSnapshotHandler(a.executionSnapshotService, secret, a.logger).RegisterRoutes(mux)
}
