"use client"

import { useState } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { Header } from "@/components/marketplace/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Checkbox } from "@/components/ui/checkbox"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { 
  ArrowLeft,
  ArrowRight,
  Check,
  Shield,
  Server,
  Gauge,
  Globe,
  Lock,
  FileCheck
} from "lucide-react"
import { cn } from "@/lib/utils"

const steps = [
  { id: 1, name: "Configuration", icon: Server },
  { id: 2, name: "Environment", icon: Globe },
  { id: 3, name: "Scaling", icon: Gauge },
  { id: 4, name: "Policies", icon: Shield },
  { id: 5, name: "Review", icon: FileCheck },
]

const regions = [
  { value: "eastus", label: "East US", latency: "12ms" },
  { value: "westus2", label: "West US 2", latency: "45ms" },
  { value: "centralus", label: "Central US", latency: "28ms" },
  { value: "westeurope", label: "West Europe", latency: "98ms" },
  { value: "eastasia", label: "East Asia", latency: "156ms" },
]

const policies = [
  { id: "rate-limit", name: "Rate Limiting", description: "Limit requests per minute per user" },
  { id: "auth", name: "Authentication Required", description: "Require valid API key or OAuth token" },
  { id: "logging", name: "Audit Logging", description: "Log all requests for compliance" },
  { id: "encryption", name: "Data Encryption", description: "Encrypt data at rest and in transit" },
  { id: "geo-restrict", name: "Geo Restrictions", description: "Restrict access by geography" },
]

export default function NewDeploymentPage() {
  const router = useRouter()
  const [currentStep, setCurrentStep] = useState(1)
  const [config, setConfig] = useState({
    name: "",
    workflow: "customer-support-pipeline",
    environment: "staging",
    region: "eastus",
    minInstances: 1,
    maxInstances: 5,
    selectedPolicies: ["auth", "logging"],
  })

  const handleNext = () => {
    if (currentStep < steps.length) {
      setCurrentStep(currentStep + 1)
    } else {
      // Deploy
      router.push("/deployments")
    }
  }

  const handleBack = () => {
    if (currentStep > 1) {
      setCurrentStep(currentStep - 1)
    }
  }

  const togglePolicy = (policyId: string) => {
    setConfig((prev) => ({
      ...prev,
      selectedPolicies: prev.selectedPolicies.includes(policyId)
        ? prev.selectedPolicies.filter((id) => id !== policyId)
        : [...prev.selectedPolicies, policyId],
    }))
  }

  return (
    <div className="min-h-screen bg-background">
      <Header />

      <main className="mx-auto max-w-4xl px-6 py-8">
        <Link
          href="/deployments"
          className="mb-6 inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Deployments
        </Link>

        <div className="mb-8">
          <h1 className="text-2xl font-semibold text-foreground">New Deployment</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Configure and deploy your workflow to production
          </p>
        </div>

        {/* Progress Steps */}
        <div className="mb-8">
          <div className="flex items-center justify-between">
            {steps.map((step, index) => {
              const StepIcon = step.icon
              const isCompleted = currentStep > step.id
              const isCurrent = currentStep === step.id

              return (
                <div key={step.id} className="flex items-center">
                  <div className="flex flex-col items-center">
                    <div
                      className={cn(
                        "flex h-10 w-10 items-center justify-center rounded-full border-2 transition-colors",
                        isCompleted && "border-accent bg-accent",
                        isCurrent && "border-accent",
                        !isCompleted && !isCurrent && "border-border"
                      )}
                    >
                      {isCompleted ? (
                        <Check className="h-5 w-5 text-accent-foreground" />
                      ) : (
                        <StepIcon
                          className={cn(
                            "h-5 w-5",
                            isCurrent ? "text-accent" : "text-muted-foreground"
                          )}
                        />
                      )}
                    </div>
                    <span
                      className={cn(
                        "mt-2 text-xs",
                        isCurrent ? "text-foreground font-medium" : "text-muted-foreground"
                      )}
                    >
                      {step.name}
                    </span>
                  </div>
                  {index < steps.length - 1 && (
                    <div
                      className={cn(
                        "mx-4 h-0.5 w-16 transition-colors",
                        currentStep > step.id ? "bg-accent" : "bg-border"
                      )}
                    />
                  )}
                </div>
              )
            })}
          </div>
        </div>

        {/* Step Content */}
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>{steps[currentStep - 1].name}</CardTitle>
            <CardDescription>
              {currentStep === 1 && "Set the basic configuration for your deployment"}
              {currentStep === 2 && "Choose the target environment and region"}
              {currentStep === 3 && "Configure auto-scaling parameters"}
              {currentStep === 4 && "Select security and compliance policies"}
              {currentStep === 5 && "Review your configuration before deploying"}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {currentStep === 1 && (
              <div className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="name">Deployment Name</Label>
                  <Input
                    id="name"
                    placeholder="e.g., Customer Support Production"
                    value={config.name}
                    onChange={(e) => setConfig({ ...config, name: e.target.value })}
                  />
                </div>
                <div className="space-y-2">
                  <Label>Workflow</Label>
                  <Select
                    value={config.workflow}
                    onValueChange={(value) => setConfig({ ...config, workflow: value })}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select workflow" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="customer-support-pipeline">Customer Support Pipeline</SelectItem>
                      <SelectItem value="code-review-workflow">Code Review Workflow</SelectItem>
                      <SelectItem value="data-etl-workflow">Data ETL Workflow</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
            )}

            {currentStep === 2 && (
              <div className="space-y-6">
                <div className="space-y-3">
                  <Label>Environment</Label>
                  <RadioGroup
                    value={config.environment}
                    onValueChange={(value) => setConfig({ ...config, environment: value })}
                    className="grid grid-cols-3 gap-4"
                  >
                    {["development", "staging", "production"].map((env) => (
                      <Label
                        key={env}
                        htmlFor={env}
                        className={cn(
                          "flex cursor-pointer items-center justify-center rounded-lg border-2 p-4 transition-colors",
                          config.environment === env
                            ? "border-accent bg-accent/10"
                            : "border-border hover:border-muted-foreground/50"
                        )}
                      >
                        <RadioGroupItem value={env} id={env} className="sr-only" />
                        <span className="capitalize">{env}</span>
                      </Label>
                    ))}
                  </RadioGroup>
                </div>

                <div className="space-y-3">
                  <Label>Region</Label>
                  <div className="grid gap-2">
                    {regions.map((region) => (
                      <Label
                        key={region.value}
                        htmlFor={region.value}
                        className={cn(
                          "flex cursor-pointer items-center justify-between rounded-lg border p-4 transition-colors",
                          config.region === region.value
                            ? "border-accent bg-accent/10"
                            : "border-border hover:border-muted-foreground/50"
                        )}
                      >
                        <div className="flex items-center gap-3">
                          <input
                            type="radio"
                            id={region.value}
                            name="region"
                            value={region.value}
                            checked={config.region === region.value}
                            onChange={() => setConfig({ ...config, region: region.value })}
                            className="sr-only"
                          />
                          <Globe className="h-4 w-4 text-muted-foreground" />
                          <span>{region.label}</span>
                        </div>
                        <span className="text-sm text-muted-foreground">{region.latency}</span>
                      </Label>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {currentStep === 3 && (
              <div className="space-y-6">
                <div className="grid gap-6 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="minInstances">Minimum Instances</Label>
                    <Input
                      id="minInstances"
                      type="number"
                      min={1}
                      max={10}
                      value={config.minInstances}
                      onChange={(e) =>
                        setConfig({ ...config, minInstances: parseInt(e.target.value) || 1 })
                      }
                    />
                    <p className="text-xs text-muted-foreground">
                      Always keep at least this many instances running
                    </p>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="maxInstances">Maximum Instances</Label>
                    <Input
                      id="maxInstances"
                      type="number"
                      min={1}
                      max={100}
                      value={config.maxInstances}
                      onChange={(e) =>
                        setConfig({ ...config, maxInstances: parseInt(e.target.value) || 5 })
                      }
                    />
                    <p className="text-xs text-muted-foreground">
                      Scale up to this many instances under load
                    </p>
                  </div>
                </div>

                <div className="rounded-lg border border-border bg-muted/30 p-4">
                  <div className="flex items-center gap-2 text-sm">
                    <Gauge className="h-4 w-4 text-accent" />
                    <span className="font-medium text-foreground">Estimated Cost</span>
                  </div>
                  <p className="mt-1 text-2xl font-semibold text-foreground">
                    ${(config.minInstances * 45).toFixed(2)} - ${(config.maxInstances * 45).toFixed(2)}
                    <span className="text-sm font-normal text-muted-foreground"> / month</span>
                  </p>
                </div>
              </div>
            )}

            {currentStep === 4 && (
              <div className="space-y-4">
                {policies.map((policy) => (
                  <Label
                    key={policy.id}
                    htmlFor={policy.id}
                    className={cn(
                      "flex cursor-pointer items-start gap-4 rounded-lg border p-4 transition-colors",
                      config.selectedPolicies.includes(policy.id)
                        ? "border-accent bg-accent/10"
                        : "border-border hover:border-muted-foreground/50"
                    )}
                  >
                    <Checkbox
                      id={policy.id}
                      checked={config.selectedPolicies.includes(policy.id)}
                      onCheckedChange={() => togglePolicy(policy.id)}
                    />
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <Lock className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-foreground">{policy.name}</span>
                      </div>
                      <p className="mt-1 text-sm text-muted-foreground">{policy.description}</p>
                    </div>
                  </Label>
                ))}
              </div>
            )}

            {currentStep === 5 && (
              <div className="space-y-6">
                <div className="rounded-lg border border-border divide-y divide-border">
                  <div className="flex justify-between p-4">
                    <span className="text-muted-foreground">Deployment Name</span>
                    <span className="font-medium text-foreground">{config.name || "Untitled"}</span>
                  </div>
                  <div className="flex justify-between p-4">
                    <span className="text-muted-foreground">Workflow</span>
                    <span className="font-medium text-foreground capitalize">
                      {config.workflow.replace(/-/g, " ")}
                    </span>
                  </div>
                  <div className="flex justify-between p-4">
                    <span className="text-muted-foreground">Environment</span>
                    <span className="font-medium text-foreground capitalize">{config.environment}</span>
                  </div>
                  <div className="flex justify-between p-4">
                    <span className="text-muted-foreground">Region</span>
                    <span className="font-medium text-foreground">
                      {regions.find((r) => r.value === config.region)?.label}
                    </span>
                  </div>
                  <div className="flex justify-between p-4">
                    <span className="text-muted-foreground">Scaling</span>
                    <span className="font-medium text-foreground">
                      {config.minInstances} - {config.maxInstances} instances
                    </span>
                  </div>
                  <div className="flex justify-between p-4">
                    <span className="text-muted-foreground">Policies</span>
                    <span className="font-medium text-foreground">
                      {config.selectedPolicies.length} selected
                    </span>
                  </div>
                </div>

                <div className="rounded-lg border border-yellow-500/20 bg-yellow-500/10 p-4">
                  <p className="text-sm text-yellow-500">
                    Deploying to {config.environment} will make this workflow accessible to{" "}
                    {config.environment === "production" ? "all users" : "authorized testers"}.
                  </p>
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Navigation */}
        <div className="flex justify-between">
          <Button variant="outline" onClick={handleBack} disabled={currentStep === 1}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back
          </Button>
          <Button onClick={handleNext}>
            {currentStep === steps.length ? "Deploy" : "Continue"}
            {currentStep < steps.length && <ArrowRight className="ml-2 h-4 w-4" />}
          </Button>
        </div>
      </main>
    </div>
  )
}
