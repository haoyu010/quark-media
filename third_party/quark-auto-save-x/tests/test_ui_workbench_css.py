from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
INDEX = ROOT / "app" / "templates" / "index.html"
LOGIN = ROOT / "app" / "templates" / "login.html"
MAIN_CSS = ROOT / "app" / "static" / "css" / "main.css"
REBUILD_CSS = ROOT / "app" / "static" / "css" / "quark-workbench-rebuild.css"


def test_rebuild_css_is_the_only_extra_visual_layer():
    index = INDEX.read_text(encoding="utf-8")
    login = LOGIN.read_text(encoding="utf-8")

    assert './static/css/main.css' in index
    assert './static/css/quark-workbench-rebuild.css?v=1.0.69' in index
    assert './static/css/quark-workbench-rebuild.css?v=1.0.69' in login
    assert './static/css/material-unified-v2.css' not in index
    assert './static/css/mui-premium-v3.css' not in index
    assert './static/css/mui-premium-v5.css' not in index
    assert './static/css/quark-redesign-v6.css' not in index
    assert './static/css/quark-redesign-v7.css' not in index
    assert './static/css/quark-redesign-v8.css' not in index
    assert index.find('./static/css/main.css') < index.find('./static/css/quark-workbench-rebuild.css')


def test_rebuild_shell_replaces_legacy_visible_navigation():
    css = REBUILD_CSS.read_text(encoding="utf-8")
    index = INDEX.read_text(encoding="utf-8")

    assert "qx-shell-remake" in index
    assert "qx-reborn-app" in index
    assert "qx-reborn-sidebar" in index
    assert "qx-reborn-topbar" in index
    assert "qx-reborn-stats" in index
    assert "qx-reborn-task-card" in index
    assert "qx-reborn-settings-panel" in index
    assert "Quark Workbench Rebuild v1" in css
    assert "#app.qx-reborn-active > nav.navbar" in css
    assert "#app.qx-reborn-active #sidebarMenu" in css
    assert "grid-template-columns: var(--qx-sidebar) minmax(0, 1fr)" in css
    assert "html body #app.qx-reborn-active main > form > .qx-reborn-app ~ div:not(.modal)" in css


def test_rebuild_task_page_does_not_use_removed_old_task_templates():
    index = INDEX.read_text(encoding="utf-8")

    assert "qx-v2-page" not in index
    assert "qx-v2-sidebar" not in index
    assert "qx-v2-task-wall" not in index
    assert "qx-v2-task-card" not in index
    assert "tasklist-workbench-page qx-home" not in index
    assert "qx-command-center" not in index
    assert "qx-v3-page" not in index
    assert "qx-v3-shell" not in index
    assert "qx-v3-rail" not in index
    assert "qx-v3-stage" not in index
    assert "task-dashboard-hero" not in index


def test_rebuild_settings_contains_all_config_sections_in_new_panel():
    index = INDEX.read_text(encoding="utf-8")

    for section in [
        "account",
        "cookie",
        "schedule",
        "notify",
        "source",
        "magic",
        "episode",
        "task",
        "display",
        "api",
    ]:
        assert f"activeConfigSection === '{section}'" in index

    assert "qx-reborn-panel-head" in index
    assert "episodePatternsText" in index
    assert "tvShowKeywordsText" in index
    assert "formData.button_display_order" in index


def test_imagegen_reimagined_shell_has_integrated_controls():
    css = REBUILD_CSS.read_text(encoding="utf-8")
    index = INDEX.read_text(encoding="utf-8")

    assert "qx-reborn-header-search" in index
    assert "qx-reborn-sidebar-section" in index
    assert "qx-reborn-sidebar-status" in index
    assert "qx-reborn-stat-icon" in index
    assert "const uiStateVersion = '1.0.69';" in index
    assert "localStorage.setItem('quarkAutoSave_activeTab', 'tasklist')" in index
    assert "localStorage.setItem('tasklist_view_mode', 'poster')" in index
    assert "localStorage.removeItem('tasklist_status_filter')" in index
    assert "this.taskStatusFilter = '';" in index
    assert "this.tasklist.viewMode = 'poster';" in index
    assert "this.tasklist.selectedType = 'all';" in index
    assert 'v{{ version }} · Web UI' not in index
    assert "localStorage.setItem('tasklist_page_size', '12')" in index
    assert "qx-reborn-status-version" in index
    assert '<span v-html="versionTips"></span></small>' in index
    assert "setActiveConfigSection('notify'); changeTab('config')" in index
    assert ".qx-reborn-header-search" in css
    assert ".qx-reborn-sidebar-section" in css
    assert ".qx-reborn-sidebar-status" in css
    assert ".qx-reborn-stat-icon" in css


def test_task_wall_matches_preview_width_on_ultrawide_screens():
    css = REBUILD_CSS.read_text(encoding="utf-8")

    assert "--qx-task-wall-max: 1420px" in css
    assert "--qx-task-card-min: 420px" in css
    assert ".qx-reborn-page > .qx-reborn-stats" in css
    assert ".qx-reborn-task-grid" in css
    assert "max-width: var(--qx-task-wall-max)" in css
    assert "grid-template-columns: repeat(auto-fit, minmax(var(--qx-task-card-min), 1fr))" in css
    assert "@media (max-width: 767.98px)" in css
    assert "--qx-task-card-min: 0px" in css


def test_task_wall_matches_target_reference_layout():
    css = REBUILD_CSS.read_text(encoding="utf-8")
    index = INDEX.read_text(encoding="utf-8")

    for section in ["account", "cookie", "schedule", "notify", "source", "magic", "task", "display", "api"]:
        assert f"setActiveConfigSection('{section}'); changeTab('config')" in index

    assert "qx-reborn-sidebar-section-title" in index
    assert "qx-reborn-filter-toggle" in index
    assert "qx-reborn-pagination" in index
    assert "tasklistPagedTasks" in index
    assert "changeTasklistPage(" in index
    assert "changeTasklistPageSize(" in index
    assert "tasklistPageNumbers" in index
    assert "pageSize:" in index
    assert "v-for=\"(task, index) in tasklistPagedTasks\"" in index

    assert ".qx-reborn-tabs button.active::after" in css
    assert ".qx-reborn-filter-toggle" in css
    assert ".qx-reborn-pagination" in css
    assert "grid-template-columns: 56px minmax(0, 1fr)" in css
    assert "background: rgba(255,255,255,0.92)" in css


def test_product_design_polish_keeps_task_wall_compact_and_accessible():
    css = REBUILD_CSS.read_text(encoding="utf-8")

    assert "Product Design polish v2" in css
    assert "--qx-focus-ring: 0 0 0 3px rgba(47, 99, 232, .18)" in css
    assert ".qx-reborn-app :is(button, a, input, select, textarea):focus-visible" in css
    assert ".qx-reborn-task-card::before" in css
    assert ".qx-reborn-card-actions button:not(.danger)" in css
    assert ".qx-reborn-card-actions button.danger" in css
    assert ".qx-reborn-task-card:hover .qx-reborn-poster img" in css
    assert "grid-template-columns: repeat(2, minmax(0, 1fr)) !important" in css
    assert ".qx-reborn-mobile-compact-stat" in css
    assert "min-height: 82px !important" in css
    assert ".qx-reborn-mobile-topbar-compact" in css
    assert "grid-template-columns: 48px minmax(0, 1fr) !important" in css
    assert ".qx-reborn-mobile-tabs-scroll" in css
    assert "flex-wrap: nowrap !important" in css
    assert "scroll-snap-type: x proximity !important" in css


def test_rebuild_mobile_breakpoints_keep_single_piece_shell_responsive():
    css = REBUILD_CSS.read_text(encoding="utf-8")

    assert "@media (max-width: 1180px)" in css
    assert "@media (max-width: 767.98px)" in css
    assert ".qx-reborn-app" in css
    assert "grid-template-columns: 1fr !important" in css
    assert ".qx-reborn-nav" in css
    assert "overflow-x: auto !important" in css
    assert ".qx-reborn-task-card" in css
    assert ".qx-reborn-card-actions" in css


def test_legacy_main_css_is_still_available_for_behavior_classes():
    css = MAIN_CSS.read_text(encoding="utf-8")

    assert "Workbench Refresh v4" in css
    assert "Mobile Tasklist Refresh v1" in css
    assert ".tasklist-workbench-page" in css
