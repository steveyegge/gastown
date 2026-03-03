// Azure Functions v4 node model — each module self-registers via app.http().
// Importing them here causes the registration to run when the host loads `main`.

export * from "./catalog/assets.js";
export * from "./catalog/ratings.js";
export * from "./catalog/versions.js";

export * from "./publisher/publishers.js";
export * from "./publisher/submissions.js";
export * from "./publisher/review.js";

export * from "./sessions/sessions.js";

export * from "./workflows/workflows.js";

export * from "./workspace/projects.js";
export * from "./workspace/user-config.js";

export * from "./data/data-sources.js";
