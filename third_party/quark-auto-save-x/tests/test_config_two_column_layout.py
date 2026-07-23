from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
INDEX = ROOT / "app" / "templates" / "index.html"
CSS = ROOT / "app" / "static" / "css" / "quark-redesign-v8.css"
REDESIGN_CSS = ROOT / "app" / "static" / "css" / "quark-redesign-v8.css"


def test_config_page_uses_settings_workbench_shell():
    index = INDEX.read_text(encoding="utf-8")
    css = CSS.read_text(encoding="utf-8")

    assert "config-workbench-page" in index
    assert "config-workbench-shell" in index
    assert "config-workbench-nav" in index
    assert "config-workbench-content" in index
    assert "config-section-card" in index
    assert "config-nav-link" in index
    assert ".config-workbench-shell" in css
    assert ".config-workbench-nav" in css
    assert ".config-workbench-content" in css
    assert ".config-section-card" in css
    assert ".config-nav-link" in css


def test_config_page_keeps_anchor_sections_without_auto_collapsing_global_sidebar():
    index = INDEX.read_text(encoding="utf-8")

    for anchor in [
        "config-account",
        "config-cookie",
        "config-schedule",
        "config-notify",
        "config-source",
        "config-magic",
        "config-episode",
        "config-task",
        "config-display",
        "config-api",
    ]:
        assert f'id="{anchor}"' in index

    assert "if (tab === 'config')" not in index
    assert "this.sidebarCollapsed = true" not in index
    assert "configSidebarWasCollapsed" not in index
    assert "configSidebarAutoManaged" not in index


def test_mobile_tab_switch_hides_sidebar_instead_of_toggling_it_open():
    index = INDEX.read_text(encoding="utf-8")

    assert "closeMobileSidebar()" in index
    assert "this.closeMobileSidebar()" in index
    assert "$('#sidebarMenu').collapse('hide')" in index
    assert "removeClass('show')" in index
    assert "this.closeMobileSidebar().removeClass" not in index
    assert "$('#sidebarMenu').collapse('toggle')" not in index


def test_config_mobile_css_keeps_global_sidebar_from_covering_settings():
    css = CSS.read_text(encoding="utf-8")

    assert "@media (max-width: 767.98px)" in css
    assert "#sidebarMenu.collapse:not(.show)" in css
    assert "display: none !important" in css


def test_config_page_reduces_nested_card_drift_in_css():
    css = CSS.read_text(encoding="utf-8")

    assert ".config-workbench-shell" in css
    assert ".config-workbench-nav" in css
    assert ".config-workbench-content" in css
    assert ".config-section-card" in css
    assert ".config-settings-page" not in css
    assert "--qx8-frame-radius: 0px" in css
    assert "background: var(--qx8-surface) !important" in css

def test_config_sidebar_does_not_duplicate_page_title():
    index = INDEX.read_text(encoding="utf-8")

    assert "config-workbench-header" in index
    assert "config-workbench-header-kicker" in index
    assert "config-workbench-nav-brand" not in index
    assert "??????" not in index

def test_config_mobile_width_guard_prevents_horizontal_expansion():
    css = CSS.read_text(encoding="utf-8")

    assert ".config-workbench-shell > *" in css
    assert ".config-workbench-nav" in css
    assert ".config-workbench-content" in css
    assert ".config-nav-list" in css
    assert "overflow-x: auto !important" in css


def test_final_redesign_layer_replaces_legacy_visual_shell():
    index = INDEX.read_text(encoding="utf-8")
    css = REDESIGN_CSS.read_text(encoding="utf-8")

    assert './static/css/quark-workbench-rebuild.css' in index
    assert './static/css/mui-premium-v5.css' not in index
    assert './static/css/quark-redesign-v7.css' not in index
    assert index.find('./static/css/main.css') < index.find('./static/css/quark-workbench-rebuild.css')
    rebuild_css = (ROOT / "app" / "static" / "css" / "quark-workbench-rebuild.css").read_text(encoding="utf-8")
    assert "Quark Workbench Rebuild v1" in rebuild_css
    assert "--qx-sidebar: 272px" in rebuild_css
    assert "#app.qx-reborn-active > nav.navbar" in rebuild_css
    assert "#app.qx-reborn-active #sidebarMenu" in rebuild_css
    assert ".qx-reborn-app" in rebuild_css
    assert ".qx-reborn-settings-panel" in rebuild_css

