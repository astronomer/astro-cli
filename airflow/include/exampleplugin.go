package include

import "strings"

// ExamplePlugin created with astro airflow init
var ExamplePlugin = strings.TrimSpace(`
from airflow.plugins_manager import AirflowPlugin
from airflow.version import version

"""
Look for the Astronomer tab in the UI.
"""
airflow_plugins_ml = {
    "name": "Airflow-Plugins",
    "category": "Astronomer",
    "category_icon": "fa-rocket",
    "href": "https://github.com/airflow-plugins/"
}

astro_docs_ml = {
    "name": "Astronomer Docs",
    "category": "Astronomer",
    "category_icon": "fa-rocket",
    "href": "https://www.astronomer.io/docs/"
}

astro_guides_ml = {
    "name": "Airflow Guide",
    "category": "Astronomer",
    "category_icon": "fa-rocket",
    "href": "https://www.astronomer.io/guides/"
}

_appbuilder_menu_items = [airflow_plugins_ml, astro_docs_ml, astro_guides_ml]

# Airflow >= 2.0
if version.startswith("2"):
    class AstroLinksPlugin(AirflowPlugin):
        name = 'astronomer_menu_links'
        operators = []
        flask_blueprints = []
        hooks = []
        executors = []
        macros = []
        admin_views = []
        appbuilder_views = []
        appbuilder_menu_items = [airflow_plugins_ml, astro_docs_ml, astro_guides_ml]
else:
    # Airflow < 2.0
    from flask_admin.base import MenuLink

    class AstroLinksPlugin(AirflowPlugin):
        name = 'astronomer_menu_links'
        operators = []
        flask_blueprints = []
        hooks = []
        executors = []
        macros = []
        admin_views = []
        menu_links = [
            MenuLink(
                category=ml.get("category"),
                name=ml.get("name"),
                url=ml.get("href")
            ) for ml in _appbuilder_menu_items
        ]
        appbuilder_views = []
        appbuilder_menu_items = _appbuilder_menu_items
`)
