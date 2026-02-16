// Strip markdown code fences from model output before assertion evaluation.
// Models frequently wrap JSON responses in ```json ... ``` despite instructions not to.
const stripped = output.replace(/^```(?:json)?\s*\n?/gm, '').replace(/\n?```\s*$/gm, '').trim();
stripped;
