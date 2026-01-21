# Imagen Project Definitions

This directory contains project definition files for batch image generation.

## Creating a New Project

1. Copy `_template.sh` to `your-project.sh`
2. Edit the file to define your items, variants, and settings
3. Run with: `imagen-batch your-project [variant]`

## Project File Structure

Each project file defines:
- `PROJECT_NAME` - Display name for the project
- `PROJECT_DESC` - Short description
- `DEFAULT_OUTPUT` - Default output directory
- `VARIANTS` - Array of variant names
- `get_items_for_variant()` - Function returning items for a variant
- `get_variant_styles()` - (Optional) Variant-specific style recommendations

## Item Format

Items are defined as pipe-separated strings:
```
"item_id|Display Name|Description for the AI prompt"
```

## Example Usage

```bash
# List available projects
imagen-batch --list

# Test styles for a project
imagen-batch weirdchess --test grand

# Generate all items for a variant
imagen-batch weirdchess --style flat grand

# Generate with custom output
imagen-batch icons --style neon ui --output ./assets/icons
```
