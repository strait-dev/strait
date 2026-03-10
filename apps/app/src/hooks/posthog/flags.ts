/**
 * PostHog feature flag keys.
 * Keep in sync with flags created in PostHog dashboard.
 */
export const FEATURE_FLAGS = {
  // Numeric limits (4 flags)
  LIMIT_STORES: "limit_stores",
  LIMIT_REGISTERS_PER_STORE: "limit_registers_per_store",
  LIMIT_TEAM_MEMBERS_PER_STORE: "limit_team_members_per_store",
  LIMIT_PRODUCTS: "limit_products",

  // Inventory features (3 flags)
  STOCK_COUNTS: "stock_counts",
  STOCK_TRANSFERS: "stock_transfers",
  RETURNS: "returns",

  // Sales features (5 flags)
  QUOTES: "quotes",
  STORE_CREDIT: "store_credit",
  GIFT_CARDS: "gift_cards",
  REGISTER_SUMMARIES: "register_summaries",
  EMPLOYEE_SALES_TRACKING: "employee_sales_tracking",

  // Export features (3 flags)
  CUSTOM_DATE_RANGE: "custom_date_range",
  EXPORT_CSV: "export_csv",
  EXPORT_EXCEL: "export_excel",

  // AI features (5 flags)
  AI_ASSISTANT: "ai_assistant",
  AI_SALES_AGENT: "ai_sales_agent",
  AI_REPORTS_AGENT: "ai_reports_agent",
  AI_INVENTORY_AGENT: "ai_inventory_agent",
  AI_MARKETING_AGENT: "ai_marketing_agent",

  // Sales Report tabs (4 flags)
  REPORT_SALES_OVERVIEW: "report_sales_overview",
  REPORT_SALES_PAYMENTS: "report_sales_payments",
  REPORT_SALES_DISCOUNTS_RETURNS: "report_sales_discounts_returns",
  REPORT_SALES_TEAM: "report_sales_team",

  // Inventory Report tabs (6 flags)
  REPORT_INVENTORY_OVERVIEW: "report_inventory_overview",
  REPORT_INVENTORY_STOCK_LEVELS: "report_inventory_stock_levels",
  REPORT_INVENTORY_MOVEMENTS: "report_inventory_movements",
  REPORT_INVENTORY_PURCHASES: "report_inventory_purchases",
  REPORT_INVENTORY_STOCK_COUNTS: "report_inventory_stock_counts",
  REPORT_INVENTORY_TURNOVER: "report_inventory_turnover",

  // Catalog Report tabs (4 flags)
  REPORT_CATALOG_PROMOTIONS: "report_catalog_promotions",
  REPORT_CATALOG_BRANDS: "report_catalog_brands",
  REPORT_CATALOG_SERVICES: "report_catalog_services",
  REPORT_CATALOG_GROUPS_TAGS: "report_catalog_groups_tags",

  // Finance Report tabs (6 flags)
  REPORT_FINANCE_OVERVIEW: "report_finance_overview",
  REPORT_FINANCE_CASH_FLOW: "report_finance_cash_flow",
  REPORT_FINANCE_PROFITABILITY: "report_finance_profitability",
  REPORT_FINANCE_EXPENSES: "report_finance_expenses",
  REPORT_FINANCE_RECEIVABLES: "report_finance_receivables",
  REPORT_FINANCE_PAYABLES: "report_finance_payables",

  // Tiered features (4 flags)
  TIER_POS_SYSTEM: "tier_pos_system",
  TIER_CUSTOMER_PROFILES: "tier_customer_profiles",
  TIER_LOYALTY_PROGRAM: "tier_loyalty_program",
  TIER_SUPPORT: "tier_support",
} as const;

export type FeatureFlagKey = (typeof FEATURE_FLAGS)[keyof typeof FEATURE_FLAGS];
