Complex Feature Plan

## Overview
A multi-step feature with complex dependencies and parallel work.

## Step: design-api
Design the API interface and data structures.
Tier: opus

## Step: implement-core
Implement the core business logic.
Needs: design-api
Tier: opus

## Step: implement-handlers
Implement HTTP handlers for the API.
Needs: design-api
Tier: sonnet

## Step: add-validation
Add input validation and error handling.
Needs: implement-core, implement-handlers
Tier: sonnet

## Step: write-tests
Write unit and integration tests.
Needs: implement-core, implement-handlers
Tier: haiku

## Step: update-docs
Update API documentation.
Needs: design-api
Tier: haiku

## Step: final-review
Final integration review and cleanup.
Needs: add-validation, write-tests, update-docs
Tier: sonnet
