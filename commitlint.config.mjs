/**
 * Conventional Commits (https://www.conventionalcommits.org/en/v1.0.0/),
 * enforced via the commit-msg hook in .husky/.
 */
export default {
  extends: ["@commitlint/config-conventional"],
  rules: {
    "scope-enum": [
      2,
      "always",
      [
        "gateway",
        "order",
        "payment",
        "proto",
        "database",
        "messaging",
        "paystack",
        "webhook",
        "idempotency",
        "outbox",
        "observability",
        "security",
        "architecture",
        "repo",
      ],
    ],
  },
};
