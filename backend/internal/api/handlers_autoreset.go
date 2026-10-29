package api

import (
	"net/http"

	"zai-zcode-reset/internal/autoreset"
	"zai-zcode-reset/internal/model"
)

// ---------- 到期自动重置配置 ----------

type autoResetAccountView struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Enabled   bool                   `json:"enabled"`
	Override  *model.AutoResetConfig `json:"override,omitempty"`
	Effective model.AutoResetConfig  `json:"effective"`
	Source    string                 `json:"source"` // override | global
}

func (s *Server) handleGetAutoReset(w http.ResponseWriter, r *http.Request) {
	global := s.store.GetAutoResetConfig()
	accounts := s.store.ListAccounts()
	out := make([]autoResetAccountView, 0, len(accounts))
	for _, a := range accounts {
		view := autoResetAccountView{
			ID:        a.ID,
			Name:      a.Name,
			Enabled:   a.Enabled,
			Override:  a.AutoReset,
			Effective: autoreset.EffectiveConfig(a.AutoReset, global),
			Source:    "global",
		}
		if a.AutoReset != nil {
			view.Source = "override"
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"global": global, "accounts": out})
}

type autoResetRequest struct {
	Enabled           *bool `json:"enabled"`
	LeadMinutes       *int  `json:"lead_minutes"`
	FiveHourThreshold *int  `json:"five_hour_threshold"`
	WeekThreshold     *int  `json:"week_threshold"`
}

func (s *Server) handlePutAutoReset(w http.ResponseWriter, r *http.Request) {
	var req autoResetRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	cfg := s.store.GetAutoResetConfig()
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.LeadMinutes != nil {
		cfg.LeadMinutes = *req.LeadMinutes
	}
	if req.FiveHourThreshold != nil {
		cfg.FiveHourThreshold = *req.FiveHourThreshold
	}
	if req.WeekThreshold != nil {
		cfg.WeekThreshold = *req.WeekThreshold
	}
	if err := cfg.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := s.store.SetAutoResetConfig(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "保存自动重置配置失败")
		return
	}
	s.audit(r, "autoreset.update", "", map[string]interface{}{
		"enabled": cfg.Enabled, "lead_minutes": cfg.LeadMinutes,
		"five_hour_threshold": cfg.FiveHourThreshold, "week_threshold": cfg.WeekThreshold,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"global": cfg})
}
