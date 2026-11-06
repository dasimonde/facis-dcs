"""API operations for the Semantic Hub search-filter configuration."""

from steps.support.api_client import get_with_headers, put_json


class SearchFilterConfigurationService:
    @staticmethod
    def retrieve(context, headers=None):
        return get_with_headers(context, f"{context.base_url}/semantic/search-filters", headers=headers)

    @staticmethod
    def save(context, configuration, headers=None):
        return put_json(context, f"{context.base_url}/semantic/search-filters", configuration, headers=headers)
