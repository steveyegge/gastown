// =============================================================================
// Azure AI Hub Workspace — Healthcare AI Marketplace
// =============================================================================
// Creates:
//   1. Storage Account (required Hub dependency)
//   2. Key Vault (required Hub dependency)
//   3. Log Analytics Workspace + Application Insights (optional but recommended)
//   4. Azure AI Hub Workspace (kind=Hub, kind!=Default — enables evaluation runs)
//   5. Role assignments: Hub MI → KV Secrets Officer; SP → Contributor on Hub
//
// After deployment, set AZURE_AI_HUB_WORKSPACE_URL in .env.local to:
//   outputs.evaluationsBaseUrl
//
// Deploy with:
//   az deployment group create \
//     --resource-group rg-ai-marketplace-dev \
//     --template-file main.bicep \
//     --parameters servicePrincipalObjectId=<SP-OBJECT-ID>
// =============================================================================

@description('Azure region for all resources.')
param location string = 'eastus'

@description('Short prefix used in all resource names.')
@maxLength(10)
param prefix string = 'aimarket'

@description('Object ID (not client ID) of the service principal that the web app uses. Granted Contributor on the Hub workspace.')
param servicePrincipalObjectId string

// ---------------------------------------------------------------------------
// Built-in role definition IDs
// ---------------------------------------------------------------------------
var contributorRoleId      = 'b24988ac-6180-42a0-ab88-20f7382dd24c'
var kvSecretsOfficerRoleId = 'b86a8fe4-44ce-4948-aee5-eccb2c155cd7'

// ---------------------------------------------------------------------------
// Resource names
// Storage accounts: 3-24 chars, lowercase alphanumeric only
// Key Vault: 3-24 chars, alphanumeric + hyphens
// ---------------------------------------------------------------------------
var storageAccountName = take(toLower('${prefix}stor${uniqueString(resourceGroup().id)}'), 24)
var keyVaultName       = take('${prefix}-kv-${uniqueString(resourceGroup().id)}', 24)
var logAnalyticsName   = '${prefix}-logs'
var appInsightsName    = '${prefix}-appins'
var hubWorkspaceName   = '${prefix}-hub'

var commonTags = {
  project:     'ai-marketplace'
  environment: 'dev'
  createdBy:   'bicep'
}

// ---------------------------------------------------------------------------
// 1. Storage Account
// ---------------------------------------------------------------------------
resource storage 'Microsoft.Storage/storageAccounts@2024-01-01' = {
  name:     storageAccountName
  location: location
  kind:     'StorageV2'
  sku: { name: 'Standard_LRS' }
  properties: {
    allowBlobPublicAccess:  false
    minimumTlsVersion:      'TLS1_2'
    supportsHttpsTrafficOnly: true
  }
  tags: commonTags
}

// ---------------------------------------------------------------------------
// 2. Key Vault (RBAC-based access, soft-delete enabled)
// ---------------------------------------------------------------------------
resource keyVault 'Microsoft.KeyVault/vaults@2023-07-01' = {
  name:     keyVaultName
  location: location
  properties: {
    sku: { family: 'A', name: 'standard' }
    tenantId:                  tenant().tenantId
    enableSoftDelete:          true
    softDeleteRetentionInDays: 7
    enableRbacAuthorization:   true   // use role assignments, not access policies
    enabledForTemplateDeployment: true
  }
  tags: commonTags
}

// ---------------------------------------------------------------------------
// 3. Log Analytics Workspace
// ---------------------------------------------------------------------------
resource logAnalytics 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
  name:     logAnalyticsName
  location: location
  properties: {
    sku: { name: 'PerGB2018' }
    retentionInDays: 30
  }
  tags: commonTags
}

// ---------------------------------------------------------------------------
// 4. Application Insights (bound to Log Analytics)
// ---------------------------------------------------------------------------
resource appInsights 'Microsoft.Insights/components@2020-02-02' = {
  name:     appInsightsName
  location: location
  kind:     'web'
  properties: {
    Application_Type:    'web'
    WorkspaceResourceId: logAnalytics.id
  }
  tags: commonTags
}

// ---------------------------------------------------------------------------
// 5. Azure AI Hub Workspace
// ---------------------------------------------------------------------------
resource hub 'Microsoft.MachineLearningServices/workspaces@2024-10-01-preview' = {
  name:     hubWorkspaceName
  location: location
  kind:     'Hub'
  sku: {
    name: 'Basic'
    tier: 'Basic'
  }
  identity: {
    type: 'SystemAssigned'
  }
  properties: {
    friendlyName:      'AI Marketplace Hub'
    description:       'Azure AI Hub workspace for the Healthcare AI Marketplace — enables live evaluation runs and model safety assessments'
    storageAccount:    storage.id
    keyVault:          keyVault.id
    applicationInsights: appInsights.id
    publicNetworkAccess: 'Enabled'   // required: web app calls evaluation API over internet
  }
  tags: commonTags
}

// ---------------------------------------------------------------------------
// 6a. Key Vault Secrets Officer → Hub workspace managed identity
//     (Hub workspace needs KV access to store/read secrets)
// ---------------------------------------------------------------------------
resource kvRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name:  guid(keyVault.id, hub.id, kvSecretsOfficerRoleId)
  scope: keyVault
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', kvSecretsOfficerRoleId)
    principalId:      hub.identity.principalId
    principalType:    'ServicePrincipal'
  }
}

// ---------------------------------------------------------------------------
// 6b. Contributor → existing service principal
//     (SP needs this to call /evaluations/runs and read workspace metadata)
// ---------------------------------------------------------------------------
resource spRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name:  guid(hub.id, servicePrincipalObjectId, contributorRoleId)
  scope: hub
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', contributorRoleId)
    principalId:      servicePrincipalObjectId
    principalType:    'ServicePrincipal'
  }
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------
output hubWorkspaceName  string = hub.name
output hubWorkspaceId    string = hub.id
output storageAccountName string = storage.name
output keyVaultName      string = keyVault.name
output appInsightsName   string = appInsights.name

// *** Copy this value into .env.local as AZURE_AI_HUB_WORKSPACE_URL ***
output evaluationsBaseUrl string = 'https://${location}.api.azureml.ms/raisvc/v1.0/subscriptions/${subscription().subscriptionId}/resourceGroups/${resourceGroup().name}/providers/Microsoft.MachineLearningServices/workspaces/${hub.name}'
