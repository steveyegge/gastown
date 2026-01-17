#!/bin/bash
# SPDX-License-Identifier: MIT
# UI Icons Project Definition
# Generate consistent icon sets for applications

PROJECT_NAME="UI Icons"
PROJECT_DESC="Application icons and UI elements"
DEFAULT_OUTPUT="./icons"

VARIANTS=(
    "navigation"
    "actions"
    "social"
    "status"
    "all"
)

RECOMMENDED_STYLES=(
    "flat"
    "neon"
    "geometric"
)

# Navigation icons
NAVIGATION_ICONS=(
    "home|Home|A home/house icon for navigation"
    "menu|Menu|A hamburger menu icon, three horizontal lines"
    "back|Back|A left-pointing arrow or chevron for back navigation"
    "forward|Forward|A right-pointing arrow for forward navigation"
    "search|Search|A magnifying glass search icon"
    "settings|Settings|A gear/cog settings icon"
)

# Action icons
ACTION_ICONS=(
    "add|Add|A plus sign icon for adding items"
    "delete|Delete|A trash can icon for deletion"
    "edit|Edit|A pencil icon for editing"
    "save|Save|A floppy disk or checkmark save icon"
    "share|Share|A share icon with branching arrows"
    "download|Download|A downward arrow with line, download icon"
    "upload|Upload|An upward arrow with line, upload icon"
)

# Social icons
SOCIAL_ICONS=(
    "user|User|A person silhouette user icon"
    "users|Users|Multiple person silhouettes, group icon"
    "chat|Chat|A speech bubble chat icon"
    "heart|Heart|A heart icon for likes/favorites"
    "star|Star|A star icon for ratings"
    "bell|Bell|A notification bell icon"
)

# Status icons
STATUS_ICONS=(
    "check|Check|A checkmark success icon"
    "error|Error|An X or exclamation error icon"
    "warning|Warning|A triangle warning icon"
    "info|Info|An information circle icon"
    "loading|Loading|A circular loading/spinner icon"
)

get_items_for_variant() {
    local variant="$1"
    
    case "$variant" in
        navigation)
            printf '%s\n' "${NAVIGATION_ICONS[@]}"
            ;;
        actions)
            printf '%s\n' "${ACTION_ICONS[@]}"
            ;;
        social)
            printf '%s\n' "${SOCIAL_ICONS[@]}"
            ;;
        status)
            printf '%s\n' "${STATUS_ICONS[@]}"
            ;;
        all)
            printf '%s\n' "${NAVIGATION_ICONS[@]}" "${ACTION_ICONS[@]}" "${SOCIAL_ICONS[@]}" "${STATUS_ICONS[@]}"
            ;;
        *)
            return 1
            ;;
    esac
}

# Icons don't need color variants
SUPPORTS_COLORS=false

PROJECT_PROMPT_SUFFIX="icon, simple, clean design, suitable for UI, scalable"
