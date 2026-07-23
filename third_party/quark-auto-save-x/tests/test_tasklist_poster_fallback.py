from pathlib import Path
import sys
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[1]
INDEX = ROOT / "app" / "templates" / "index.html"
RUN = ROOT / "app" / "run.py"
APP_DIR = ROOT / "app"
if str(APP_DIR) not in sys.path:
    sys.path.insert(0, str(APP_DIR))


def test_tasklist_poster_falls_back_to_task_tmdb_metadata():
    index = INDEX.read_text(encoding="utf-8")

    assert "getTasklistPosterLikeEpisode(task)" in index
    assert "taskPosterTmdbId" in index
    assert "((task.calendar_info || {}).match || {}).tmdb_id" in index
    assert "if (calPoster && calPoster.poster_local_path) return calPoster;" in index
    assert "if (calTask) return this.getTaskPosterLikeEpisode(calTask);" not in index


def test_missing_local_poster_uses_backend_tmdb_poster_image_route():
    index = INDEX.read_text(encoding="utf-8")
    run_py = RUN.read_text(encoding="utf-8")

    assert "/api/calendar/poster_image/" in index
    assert '@app.route("/api/calendar/poster_image/<int:tmdb_id>")' in run_py
    assert "resolve_calendar_poster_image_path" in run_py


def test_poster_image_route_redirects_to_resolved_cached_poster():
    from app import run as run_module

    original_config = run_module.config_data
    run_module.config_data = {"webui": {"username": "admin", "password": "admin"}}
    client = run_module.app.test_client()
    token = run_module.get_login_token()

    try:
        with patch.object(run_module, "resolve_calendar_poster_image_path", return_value="/cache/images/test.jpg"):
            response = client.get(
                "/api/calendar/poster_image/235643",
                query_string={"token": token, "type": "anime", "name": "逆天邪神"},
            )
    finally:
        run_module.config_data = original_config

    assert response.status_code == 302
    assert response.headers["Location"] == "/cache/images/test.jpg"
