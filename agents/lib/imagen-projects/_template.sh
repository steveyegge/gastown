#!/bin/bash
# SPDX-License-Identifier: MIT
# Project Template for imagen-batch
# Copy this file and customize for your project
#
# Required:
#   PROJECT_NAME, VARIANTS, get_items_for_variant()
#
# Optional:
#   PROJECT_DESC, DEFAULT_OUTPUT, RECOMMENDED_STYLES, get_variant_styles()

# Project metadata
PROJECT_NAME="My Project"
PROJECT_DESC="Description of what this project generates"
DEFAULT_OUTPUT="./generated"

# Available variants for this project
# Users will select one of these when running imagen-batch
VARIANTS=(
    "variant1"
    "variant2"
    "all"
)

# Recommended styles for this project (shown in --test mode)
# These are the styles that will be tested
RECOMMENDED_STYLES=(
    "flat"
    "3d"
    "pixel"
)

# Define items for each variant
# Format: "item_id|Display Name|Description for AI prompt"

VARIANT1_ITEMS=(
    "item1|Item One|A detailed description for the AI to generate"
    "item2|Item Two|Another detailed description"
)

VARIANT2_ITEMS=(
    "item3|Item Three|Description for variant 2 item"
)

# Common items shared across variants (optional)
COMMON_ITEMS=(
    "common1|Common Item|Shared across all variants"
)

# Required function: returns items array for a given variant
get_items_for_variant() {
    local variant="$1"
    
    case "$variant" in
        variant1)
            echo "${COMMON_ITEMS[@]}" "${VARIANT1_ITEMS[@]}"
            ;;
        variant2)
            echo "${COMMON_ITEMS[@]}" "${VARIANT2_ITEMS[@]}"
            ;;
        all)
            echo "${COMMON_ITEMS[@]}" "${VARIANT1_ITEMS[@]}" "${VARIANT2_ITEMS[@]}"
            ;;
        *)
            echo ""
            ;;
    esac
}

# Optional function: returns recommended styles for a variant
# If not defined, uses RECOMMENDED_STYLES
get_variant_styles() {
    local variant="$1"
    
    case "$variant" in
        variant1)
            echo "flat 3d pixel"
            ;;
        variant2)
            echo "neon geometric watercolor"
            ;;
        *)
            echo "${RECOMMENDED_STYLES[*]}"
            ;;
    esac
}

# Optional: Additional prompt modifiers for this project
# Gets appended to every item description
PROJECT_PROMPT_SUFFIX=""

# Optional: Color/variation support
# Set to "true" if items should be generated in multiple color variants
SUPPORTS_COLORS=false

# Define available colors (space-separated)
# Used when --color all is specified
DEFINED_COLORS="red blue green yellow purple orange"

# Default color selection
# Can be: "all", "none", a palette name, or space-separated colors
DEFAULT_COLORS="all"

# Optional: Define named palettes
# Returns space-separated color list for a palette name
get_palette() {
    local palette="$1"
    case "$palette" in
        primary)   echo "red blue yellow" ;;
        secondary) echo "green orange purple" ;;
        warm)      echo "red orange yellow" ;;
        cool)      echo "blue green purple" ;;
        mono)      echo "black white gray" ;;
        neon)      echo "hotpink cyan lime yellow" ;;
        earth)     echo "brown tan olive sienna" ;;
        pastel)    echo "pink lightblue mint lavender peach" ;;
        *)         echo "" ;;  # Unknown palette, return empty
    esac
}

# Optional: Custom color descriptions for prompts
# More descriptive than just "red colored"
get_color_desc() {
    local color="$1"
    case "$color" in
        red)      echo "vibrant red colored" ;;
        blue)     echo "deep blue colored" ;;
        green)    echo "rich green colored" ;;
        gold)     echo "metallic gold colored" ;;
        silver)   echo "shimmering silver colored" ;;
        *)        echo "$color colored" ;;
    esac
}

