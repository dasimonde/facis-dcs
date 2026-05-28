from behave import given, when, then
import requests



#the user navigates to the frontend URL
@when('the user navigates to the frontend URL')
def step_navigate_frontend(context):
    url = "/".join(context.base_url.split("/", 3)[:3]) + '/digital-contracting-service/ui/'
    context.requests_response = requests.get(url)