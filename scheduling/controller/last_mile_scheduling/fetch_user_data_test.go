package last_mile_scheduling

import (
	"scheduling/middleware"
	"testing"
)

func TestFetchUserData(t *testing.T) {
	cfg, _ := middleware.LoadConfig("../../scheduling_config.toml")
	db := middleware.ConnectToDB(cfg.Database)
	FetchUserData(db)
}
