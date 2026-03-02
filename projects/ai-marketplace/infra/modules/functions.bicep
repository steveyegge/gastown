@description('Function app name')
param appName string

@description('Azure region')
param location string

@description('Storage account name for Functions runtime')
param storageAccountName string

@description('Application Insights instrumentation key')
param appInsightsInstrumentationKey string

@description('Cosmos DB endpoint')
param cosmosEndpoint string

@description('Cosmos DB primary key')
@secure()
param cosmosKey string

resource storageAccount 'Microsoft.Storage/storageAccounts@2023-05-01' existing = {
  name: storageAccountName
}

resource hostingPlan 'Microsoft.Web/serverfarms@2023-12-01' = {
  name: '${appName}-plan'
  location: location
  sku: {
    name: 'FC1'  // Flex Consumption
    tier: 'FlexConsumption'
  }
  properties: {
    reserved: true  // Linux
  }
}

resource functionApp 'Microsoft.Web/sites@2023-12-01' = {
  name: appName
  location: location
  kind: 'functionapp,linux'
  properties: {
    serverFarmId: hostingPlan.id
    siteConfig: {
      linuxFxVersion: 'NODE|20'
      appSettings: [
        { name: 'AzureWebJobsStorage', value: 'DefaultEndpointsProtocol=https;AccountName=${storageAccountName};AccountKey=${storageAccount.listKeys().keys[0].value}' }
        { name: 'FUNCTIONS_EXTENSION_VERSION', value: '~4' }
        { name: 'FUNCTIONS_WORKER_RUNTIME', value: 'node' }
        { name: 'WEBSITE_NODE_DEFAULT_VERSION', value: '~20' }
        { name: 'APPINSIGHTS_INSTRUMENTATIONKEY', value: appInsightsInstrumentationKey }
        { name: 'COSMOS_ENDPOINT', value: cosmosEndpoint }
        { name: 'COSMOS_KEY', value: cosmosKey }
        { name: 'COSMOS_DATABASE', value: 'ai-marketplace' }
      ]
      cors: {
        allowedOrigins: ['*']  // Tighten in production
        supportCredentials: false
      }
    }
    httpsOnly: true
  }
}

output defaultHostName string = 'https://${functionApp.properties.defaultHostName}'
output functionAppName string = functionApp.name
