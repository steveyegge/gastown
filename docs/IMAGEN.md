# Imagen - AI Image Generation

Imagen is Gastown's image generation system using Google's Gemini 2.0 Flash model. It supports single image generation and batch generation with project templates.

## Requirements

- **curl** - For API requests
- **jq** - For JSON parsing
- **base64** - For image encoding/decoding (typically pre-installed on macOS/Linux)
- **Gemini API Key** - Set as `GEMINI_API_KEY` or `GOOGLE_API_KEY`

## Quick Start

```bash
# Generate a single image
./agents/imagen "a cute robot mascot"

# Generate with a style preset
./agents/imagen --style pixel "a knight chess piece"

# List available projects
./agents/imagen-batch --list

# Generate test images in different styles
./agents/imagen-batch icons --test navigation
```

## Single Image Generation

The `imagen` agent generates individual images from text prompts.

### Basic Usage

```bash
./agents/imagen "description of what to generate"
```

### Options

| Option | Description |
|--------|-------------|
| `--style <name>` | Apply a style preset |
| `--aspect <ratio>` | Aspect ratio (square, portrait, landscape, wide, 4:3, 16:9, etc.) |
| `--size <WxH>` | Explicit dimensions (e.g., 1024x768) |
| `--reference <file>` | Reference image for style matching (can use multiple times) |
| `--output <dir>` | Output directory (default: current) |
| `--piece <name>` | Name for the output file |
| `--variant <name>` | Variant name (for batch organization) |
| `--count <n>` | Generate multiple variations |

### Style Presets

| Style | Description |
|-------|-------------|
| `flat` | Minimalist flat design, solid colors, no shadows |
| `3d` | Three-dimensional rendering with lighting |
| `pixel` | Pixel art / 8-bit style |
| `geometric` | Abstract geometric shapes |
| `medieval` | Hand-painted medieval manuscript style |
| `neon` | Glowing neon cyberpunk style |
| `watercolor` | Soft watercolor painting |
| `classic` | Traditional realistic style |
| `staunton` | Classic Staunton chess piece style |

### Aspect Ratios

| Preset | Ratio | Use Case |
|--------|-------|----------|
| `square` | 1:1 | Icons, avatars, social media |
| `portrait` | 2:3 | Mobile screens, posters |
| `landscape` | 3:2 | Photos, cards |
| `wide` | 16:9 | Cinematic, video thumbnails |
| `ultrawide` | 21:9 | Banners, headers |
| `4:3` | 4:3 | Classic photos, presentations |

You can also specify custom ratios like `3:4`, `5:4`, or `1:2`.

### Reference Images (Style Guides)

Use reference images to match a specific style, color palette, or aesthetic:

```bash
# Single reference image
./agents/imagen --reference style-guide.png "a warrior character"

# Multiple reference images (style + color palette)
./agents/imagen --reference style.png --reference palette.png "game character"

# Reference + style preset (reference takes priority)
./agents/imagen --reference example.png --style flat "matching icon"
```

**Tips for reference images:**
- Use clear, representative examples of your desired style
- Combine multiple references for complex styles (one for linework, one for colors)
- The AI will attempt to match the overall aesthetic, not copy exactly
- Works best with consistent, clean reference images

### Image Dimensions

**Note:** Gemini 2.0 Flash generates images at a fixed resolution. The `--aspect` and `--size` options add guidance to the prompt, which influences composition but doesn't guarantee exact dimensions. For precise sizes, post-process the generated images.

```bash
# Request landscape composition
./agents/imagen --aspect landscape "mountain panorama"

# Request specific dimensions (guidance only)
./agents/imagen --size 1200x800 "product photo"

# Combine with style
./agents/imagen --aspect wide --style cinematic "epic battle scene"
```

### Examples

```bash
# Pixel art robot
./agents/imagen --style pixel "a friendly robot waving"

# Multiple variations
./agents/imagen --count 3 --style flat "app icon for a music player"

# Specific output location
./agents/imagen --output ./assets --piece logo "company logo, minimalist"

# Match a reference style
./agents/imagen --reference brand-style.png "new product icon"

# Wide cinematic image
./agents/imagen --aspect wide "sunset over mountains, dramatic lighting"
```

## Batch Generation with Projects

The `imagen-batch` agent generates multiple images using project templates.

### Project Structure

Projects define:
- **Variants** - Different categories or themes
- **Items** - What to generate within each variant
- **Colors/Palettes** - Optional color variations
- **Styles** - Recommended visual styles

### Commands

```bash
# List available projects
./agents/imagen-batch --list

# Show variants for a project
./agents/imagen-batch <project> --variants

# Generate test images (one item × multiple styles)
./agents/imagen-batch <project> --test <variant>

# Generate all items in a style
./agents/imagen-batch <project> --style <style> <variant>

# Dry run (show what would be generated)
./agents/imagen-batch <project> --style flat <variant> --dry-run
```

### Color Support

Projects can define color variants for items:

```bash
# Use a named palette
./agents/imagen-batch game-chars --style pixel heroes --color fire

# Custom color list
./agents/imagen-batch game-chars --style pixel heroes --color "red blue green"

# Hex colors
./agents/imagen-batch game-chars --style pixel heroes --color "#FF5500 #00AA33"

# All defined colors
./agents/imagen-batch game-chars --style pixel heroes --color all
```

#### Named Palettes (project-defined)

Projects can define palettes like:
- `fire` → red, orange, yellow
- `ice` → blue, cyan, white
- `team-red` → red, crimson, maroon
- `legendary` → orange, gold

#### Hex Color Support

Hex colors are automatically converted to descriptive prompts:
- `#FF5500` → "orange/yellow colored (hex #FF5500)"
- `#00AA33` → "dark cyan/teal colored (hex #00AA33)"
- `F00` → "red colored (hex #FF0000)" (3-digit supported)

### File Versioning

Imagen never overwrites existing files. If a file exists, it adds version numbers:

```
knight.png        # First generation
knight_v2.png     # Second generation
knight_v3.png     # Third generation
```

## Creating Your Own Project

1. Copy the template:
   ```bash
   cp agents/lib/imagen-projects/_template.sh agents/lib/imagen-projects/myproject.sh
   ```

2. Edit the file to define:
   ```bash
   PROJECT_NAME="My Project"
   PROJECT_DESC="Description"
   DEFAULT_OUTPUT="./output"
   
   VARIANTS=("variant1" "variant2" "all")
   
   VARIANT1_ITEMS=(
       "item1|Item One|A detailed description for the AI"
       "item2|Item Two|Another description"
   )
   
   get_items_for_variant() {
       case "$1" in
           variant1) printf '%s\n' "${VARIANT1_ITEMS[@]}" ;;
           # ...
       esac
   }
   ```

3. Make it executable:
   ```bash
   chmod +x agents/lib/imagen-projects/myproject.sh
   ```

4. Use it:
   ```bash
   ./agents/imagen-batch myproject --test variant1
   ```

See [agents/lib/imagen-projects/_template.sh](../agents/lib/imagen-projects/_template.sh) for the full template with all options.

## Environment Setup

Requires a Gemini API key:

```bash
export GEMINI_API_KEY="your-key"
# or
export GOOGLE_API_KEY="your-key"
```

## Example: UI Icons Project

The included `icons` project demonstrates the system:

```bash
# Test different styles for navigation icons
./agents/imagen-batch icons --test navigation

# Generate all navigation icons in flat style
./agents/imagen-batch icons --style flat navigation

# Generate all icon categories
./agents/imagen-batch icons --style neon all
```

## Troubleshooting

### "API key required"
Set `GEMINI_API_KEY` or `GOOGLE_API_KEY` environment variable.

### "No image in response"
The model may have returned text instead of an image. Check if:
- The prompt is clear and specific
- The content doesn't violate safety guidelines
- Try adding more descriptive details

### Images look wrong
- Try different style presets
- Add more specific details to the prompt
- Use `--count 3` to generate variations and pick the best
