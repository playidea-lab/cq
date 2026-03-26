package handlers

import "github.com/changmin/c4-core/internal/mcp"

// RegisterLegacyAliases registers backward-compatibility aliases so that
// callers using the old "c4_xxx" tool names are transparently routed to the
// renamed "cq_xxx" canonical tools.
//
// This allows existing integrations and stored prompts that reference c4_*
// names to keep working without modification.
func RegisterLegacyAliases(reg *mcp.Registry) {
	aliases := [][2]string{
		// core task/state tools
		{"c4_status", "cq_status"},
		{"c4_start", "cq_start"},
		{"c4_claim", "cq_claim"},
		{"c4_submit", "cq_submit"},
		{"c4_report", "cq_report"},
		{"c4_get_task", "cq_get_task"},
		{"c4_task_list", "cq_task_list"},
		{"c4_add_todo", "cq_add_todo"},
		{"c4_mark_blocked", "cq_mark_blocked"},
		{"c4_reset_task", "cq_reset_task"},
		{"c4_stale_tasks", "cq_stale_tasks"},
		{"c4_request_changes", "cq_request_changes"},
		{"c4_task_events", "cq_task_events"},
		{"c4_task_graph", "cq_task_graph"},
		{"c4_checkpoint", "cq_checkpoint"},
		{"c4_snapshot", "cq_snapshot"},
		{"c4_reflect", "cq_reflect"},
		{"c4_recall", "cq_recall"},
		{"c4_clear", "cq_clear"},
		{"c4_whoami", "cq_whoami"},
		{"c4_worker_heartbeat", "cq_worker_heartbeat"},
		{"c4_worker_complete", "cq_worker_complete"},
		{"c4_worker_shutdown", "cq_worker_shutdown"},
		{"c4_worker_standby", "cq_worker_standby"},
		{"c4_ensure_supervisor", "cq_ensure_supervisor"},
		{"c4_record_gate", "cq_record_gate"},
		// file/code tools
		{"c4_read_file", "cq_read_file"},
		{"c4_find_file", "cq_find_file"},
		{"c4_search_for_pattern", "cq_search_for_pattern"},
		{"c4_replace_content", "cq_replace_content"},
		{"c4_create_text_file", "cq_create_text_file"},
		{"c4_list_dir", "cq_list_dir"},
		{"c4_execute", "cq_execute"},
		// git/diff tools
		{"c4_diff_summary", "cq_diff_summary"},
		{"c4_worktree_status", "cq_worktree_status"},
		{"c4_worktree_cleanup", "cq_worktree_cleanup"},
		{"c4_analyze_history", "cq_analyze_history"},
		{"c4_search_commits", "cq_search_commits"},
		// discovery/spec tools
		{"c4_discovery_complete", "cq_discovery_complete"},
		{"c4_design_complete", "cq_design_complete"},
		{"c4_get_spec", "cq_get_spec"},
		{"c4_save_spec", "cq_save_spec"},
		{"c4_list_specs", "cq_list_specs"},
		{"c4_get_design", "cq_get_design"},
		{"c4_save_design", "cq_save_design"},
		{"c4_list_designs", "cq_list_designs"},
		// validation/run tools
		{"c4_run_validation", "cq_run_validation"},
		{"c4_run_checkpoint", "cq_run_checkpoint"},
		{"c4_run_complete", "cq_run_complete"},
		{"c4_run_should_continue", "cq_run_should_continue"},
		// health
		{"c4_health", "cq_health"},
		// lighthouse/dashboard
		{"c4_lighthouse", "cq_lighthouse"},
		{"c4_dashboard", "cq_dashboard"},
		// intelligence/nodes
		{"c4_intelligence_stats", "cq_intelligence_stats"},
		{"c4_nodes_map", "cq_nodes_map"},
		{"c4_collective_stats", "cq_collective_stats"},
		{"c4_collective_sync", "cq_collective_sync"},
		{"c4_error_trace", "cq_error_trace"},
		// phase lock
		{"c4_phase_lock_acquire", "cq_phase_lock_acquire"},
		{"c4_phase_lock_release", "cq_phase_lock_release"},
		// soul/persona
		{"c4_soul_get", "cq_soul_get"},
		{"c4_soul_set", "cq_soul_set"},
		{"c4_soul_resolve", "cq_soul_resolve"},
		{"c4_persona_learn", "cq_persona_learn"},
		{"c4_persona_learn_from_diff", "cq_persona_learn_from_diff"},
		{"c4_persona_evolve", "cq_persona_evolve"},
		{"c4_persona_stats", "cq_persona_stats"},
		{"c4_profile_save", "cq_profile_save"},
		{"c4_profile_load", "cq_profile_load"},
		// LLM
		{"c4_llm_call", "cq_llm_call"},
		{"c4_llm_providers", "cq_llm_providers"},
		{"c4_llm_costs", "cq_llm_costs"},
		{"c4_llm_usage_stats", "cq_llm_usage_stats"},
		// CDP
		{"c4_cdp_run", "cq_cdp_run"},
		{"c4_cdp_list", "cq_cdp_list"},
		{"c4_cdp_action", "cq_cdp_action"},
		// web/doc tools
		{"c4_web_fetch", "cq_web_fetch"},
		{"c4_extract_text", "cq_extract_text"},
		{"c4_parse_document", "cq_parse_document"},
		{"c4_webmcp_discover", "cq_webmcp_discover"},
		{"c4_webmcp_call", "cq_webmcp_call"},
		{"c4_webmcp_context", "cq_webmcp_context"},
		// workspace
		{"c4_workspace_create", "cq_workspace_create"},
		{"c4_workspace_save", "cq_workspace_save"},
		{"c4_workspace_load", "cq_workspace_load"},
		// pop
		{"c4_pop_extract", "cq_pop_extract"},
		{"c4_pop_reflect", "cq_pop_reflect"},
		{"c4_pop_status", "cq_pop_status"},
		// LSP/symbol tools
		{"c4_find_symbol", "cq_find_symbol"},
		{"c4_get_symbols_overview", "cq_get_symbols_overview"},
		{"c4_replace_symbol_body", "cq_replace_symbol_body"},
		{"c4_insert_before_symbol", "cq_insert_before_symbol"},
		{"c4_insert_after_symbol", "cq_insert_after_symbol"},
		{"c4_rename_symbol", "cq_rename_symbol"},
		{"c4_find_referencing_symbols", "cq_find_referencing_symbols"},
		{"c4_onboard", "cq_onboard"},
		// knowledge
		{"c4_knowledge_search", "cq_knowledge_search"},
		{"c4_knowledge_record", "cq_knowledge_record"},
		{"c4_knowledge_get", "cq_knowledge_get"},
		{"c4_knowledge_ingest", "cq_knowledge_ingest"},
		{"c4_knowledge_ingest_paper", "cq_knowledge_ingest_paper"},
		{"c4_knowledge_delete", "cq_knowledge_delete"},
		{"c4_knowledge_distill", "cq_knowledge_distill"},
		{"c4_knowledge_publish", "cq_knowledge_publish"},
		{"c4_knowledge_pull", "cq_knowledge_pull"},
		{"c4_knowledge_reindex", "cq_knowledge_reindex"},
		{"c4_knowledge_stats", "cq_knowledge_stats"},
		{"c4_knowledge_discover", "cq_knowledge_discover"},
		// experiment
		{"c4_experiment_record", "cq_experiment_record"},
		{"c4_experiment_search", "cq_experiment_search"},
		{"c4_experiment_register", "cq_experiment_register"},
		{"c4_pattern_suggest", "cq_pattern_suggest"},
		// research
		{"c4_research_start", "cq_research_start"},
		{"c4_research_status", "cq_research_status"},
		{"c4_research_record", "cq_research_record"},
		{"c4_research_approve", "cq_research_approve"},
		{"c4_research_next", "cq_research_next"},
		{"c4_research_suggest", "cq_research_suggest"},
		{"c4_research_loop_start", "cq_research_loop_start"},
		{"c4_research_loop_stop", "cq_research_loop_stop"},
		// GPU
		{"c4_gpu_status", "cq_gpu_status"},
		// job
		{"c4_job_submit", "cq_job_submit"},
		{"c4_job_status", "cq_job_status"},
		{"c4_job_list", "cq_job_list"},
		{"c4_job_cancel", "cq_job_cancel"},
		{"c4_job_summary", "cq_job_summary"},
		// hub
		{"c4_hub_submit", "cq_hub_submit"},
		{"c4_hub_status", "cq_hub_status"},
		{"c4_hub_list", "cq_hub_list"},
		{"c4_hub_cancel", "cq_hub_cancel"},
		{"c4_hub_metrics", "cq_hub_metrics"},
		{"c4_hub_log_metrics", "cq_hub_log_metrics"},
		{"c4_hub_watch", "cq_hub_watch"},
		{"c4_hub_summary", "cq_hub_summary"},
		{"c4_hub_retry", "cq_hub_retry"},
		{"c4_hub_estimate", "cq_hub_estimate"},
		{"c4_hub_lease_renew", "cq_hub_lease_renew"},
		{"c4_hub_workers", "cq_hub_workers"},
		{"c4_hub_stats", "cq_hub_stats"},
		{"c4_hub_upload", "cq_hub_upload"},
		{"c4_hub_download", "cq_hub_download"},
		// hub DAG
		{"c4_hub_dag_create", "cq_hub_dag_create"},
		{"c4_hub_dag_add_node", "cq_hub_dag_add_node"},
		{"c4_hub_dag_add_dep", "cq_hub_dag_add_dep"},
		{"c4_hub_dag_execute", "cq_hub_dag_execute"},
		{"c4_hub_dag_status", "cq_hub_dag_status"},
		{"c4_hub_dag_list", "cq_hub_dag_list"},
		{"c4_hub_dag_from_yaml", "cq_hub_dag_from_yaml"},
		// cron
		{"c4_cron_create", "cq_cron_create"},
		{"c4_cron_list", "cq_cron_list"},
		{"c4_cron_delete", "cq_cron_delete"},
		// drive
		{"c4_drive_upload", "cq_drive_upload"},
		{"c4_drive_download", "cq_drive_download"},
		{"c4_drive_list", "cq_drive_list"},
		{"c4_drive_delete", "cq_drive_delete"},
		{"c4_drive_info", "cq_drive_info"},
		{"c4_drive_mkdir", "cq_drive_mkdir"},
		{"c4_drive_dataset_upload", "cq_drive_dataset_upload"},
		{"c4_drive_dataset_list", "cq_drive_dataset_list"},
		{"c4_drive_dataset_pull", "cq_drive_dataset_pull"},
		// artifact
		{"c4_artifact_save", "cq_artifact_save"},
		{"c4_artifact_list", "cq_artifact_list"},
		{"c4_artifact_get", "cq_artifact_get"},
		// event/rule
		{"c4_event_list", "cq_event_list"},
		{"c4_event_publish", "cq_event_publish"},
		{"c4_rule_add", "cq_rule_add"},
		{"c4_rule_list", "cq_rule_list"},
		{"c4_rule_remove", "cq_rule_remove"},
		{"c4_rule_toggle", "cq_rule_toggle"},
		{"c4_notification_channels", "cq_notification_channels"},
		// observe
		{"c4_observe_metrics", "cq_observe_metrics"},
		{"c4_observe_logs", "cq_observe_logs"},
		{"c4_observe_config", "cq_observe_config"},
		{"c4_observe_health", "cq_observe_health"},
		{"c4_observe_policy", "cq_observe_policy"},
		{"c4_observe_traces", "cq_observe_traces"},
		{"c4_observe_trace_stats", "cq_observe_trace_stats"},
		// gate
		{"c4_gate_webhook_register", "cq_gate_webhook_register"},
		{"c4_gate_webhook_list", "cq_gate_webhook_list"},
		{"c4_gate_webhook_test", "cq_gate_webhook_test"},
		{"c4_gate_schedule_add", "cq_gate_schedule_add"},
		{"c4_gate_schedule_list", "cq_gate_schedule_list"},
		{"c4_gate_connector_status", "cq_gate_connector_status"},
		// guard
		{"c4_guard_check", "cq_guard_check"},
		{"c4_guard_audit", "cq_guard_audit"},
		{"c4_guard_policy_set", "cq_guard_policy_set"},
		{"c4_guard_policy_list", "cq_guard_policy_list"},
		{"c4_guard_role_assign", "cq_guard_role_assign"},
		// mail
		{"c4_mail_send", "cq_mail_send"},
		{"c4_mail_ls", "cq_mail_ls"},
		{"c4_mail_read", "cq_mail_read"},
		{"c4_mail_rm", "cq_mail_rm"},
		// config/secret/notify
		{"c4_config_get", "cq_config_get"},
		{"c4_config_set", "cq_config_set"},
		{"c4_secret_set", "cq_secret_set"},
		{"c4_secret_get", "cq_secret_get"},
		{"c4_secret_list", "cq_secret_list"},
		{"c4_secret_delete", "cq_secret_delete"},
		{"c4_notification_set", "cq_notification_set"},
		{"c4_notification_get", "cq_notification_get"},
		{"c4_notify", "cq_notify"},
		// skill eval
		{"c4_skill_eval_generate", "cq_skill_eval_generate"},
		{"c4_skill_eval_status", "cq_skill_eval_status"},
		{"c4_skill_optimize", "cq_skill_optimize"},
		{"c4_skill_eval_run", "cq_skill_eval_run"},
	}

	for _, pair := range aliases {
		reg.RegisterAlias(pair[0], pair[1])
	}
}
