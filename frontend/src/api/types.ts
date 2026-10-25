export type Product = 'KASKO' | 'TRAFIK' | 'KONUT'
export type QuoteStatus = 'CALCULATED' | 'UW_REVIEW' | 'APPROVED' | 'REJECTED' | 'CONVERTED' | 'EXPIRED'

export interface Page<T> {
  items: T[]
  next_cursor: string | null
}

export interface Customer {
  id: number
  full_name: string
  national_id_masked: string
  birth_date: string
  region_code: string
  agent_id: number | null
}

export interface Factor {
  type: 'AGE' | 'NCD' | 'REGION' | 'VEHICLE_GROUP'
  key?: string
  range?: { lo: number; hi: number | null }
  multiplier_bp: number
}

export interface CoverageSpec {
  code: string
  name: string
  required: boolean
  limit_kurus: number
  deductible_type: 'NONE' | 'FIXED' | 'PERCENT_BP'
  deductible_value: number
  base_premium_kurus?: number
  premium_kurus?: number
}

export interface Tariff {
  id: number
  product: Product
  version_no: number
  valid_from: string
  valid_to: string | null
  factors: Factor[]
  coverages: CoverageSpec[]
  annual_limit_kurus: number
}

export interface PremiumLine {
  seq: number
  coverage_code: string
  line_type: 'BASE' | 'FACTOR' | 'ROUNDING'
  factor_type?: string
  factor_label: string
  multiplier_bp?: number
  amount_kurus: number
}

export interface QuoteRequest {
  customer_id: number
  product: Product
  term_start: string
  ncd_level: number
  vehicle_group?: string
  plate?: string
  coverages: string[]
}

export interface Calculation {
  tariff: { id: number; version_no: number; valid_from: string }
  term_start: string
  term_end: string
  selection: { coverages: CoverageSpec[]; annual_limit_kurus: number }
  lines: PremiumLine[]
  total_premium_kurus: number
  input_hash: string
  review_required: boolean
  review_reasons: string[]
}

export interface Quote {
  id: number
  customer_id: number
  customer_name: string
  product: Product
  tariff: { id: number; version_no: number }
  term_start: string
  term_end: string
  risk: { driver_age?: number; ncd_level?: number; region_code?: string; vehicle_group?: string; plate?: string; review_reasons?: string[] }
  selection: { coverages: CoverageSpec[]; annual_limit_kurus: number }
  input_hash: string
  total_premium_kurus: number
  status: QuoteStatus
  underwriter_note: string | null
  valid_until: string | null
  created_at: string
  policy_id: number | null
  lines?: PremiumLine[]
}

export interface PolicyItem {
  id: number
  policy_no: string
  product: Product
  customer_id: number
  customer_name: string
  start_date: string
  end_date: string
  status: string
  annual_premium_kurus: number
  vehicle_plate: string | null
}

export interface IssuedPolicy {
  id: number
  policy_no: string
  start_date: string
  end_date: string
  annual_premium_kurus: number
}

export interface VersionView {
  id: number
  version_no: number
  effective_from: string
  effective_to: string | null
  tariff_version_id: number
  tariff_version_no: number
  vehicle_plate: string | null
  risk: { driver_age?: number; ncd_level?: number; region_code?: string; vehicle_group?: string; plate?: string }
  annual_premium_kurus: number
  annual_limit_kurus: number | null
  endorsement_no: number | null
  coverages: CoverageSpec[]
}

export interface CalcDetail {
  kind: 'VEHICLE_CHANGE' | 'CANCELLATION'
  method: 'PRO_RATA' | 'SHORT_PERIOD'
  reason?: string
  period_days: number
  leap_day_in_period: boolean
  elapsed_days: number
  remaining_days: number
  premium_delta_kurus: number
  refund_kurus?: number
  version_closed?: boolean
  note?: string
  steps: string[]
}

export interface EndorsementView {
  id: number
  endorsement_no: number
  type: 'VEHICLE_CHANGE' | 'CANCELLATION' | 'COVERAGE_CHANGE' | 'OTHER'
  effective_date: string
  from_version_id: number
  to_version_id: number | null
  premium_delta_kurus: number
  calc_detail: CalcDetail
  created_at: string
}

export interface LimitBalance {
  coverage_code: string | null
  period_start: string
  period_end: string
  limit_kurus: number
  used_kurus: number
}

export interface PolicyDetail {
  id: number
  policy_no: string
  product: Product
  customer_id: number
  customer_name: string
  start_date: string
  end_date: string
  period_days: number
  status: 'ACTIVE' | 'CANCELLED' | 'EXPIRED'
  cancel_effective_date: string | null
  cancel_reason: string | null
  premium: {
    initial_premium_kurus: number
    endorsement_deltas_kurus: number
    written_premium_kurus: number
    cancellation_refund_kurus: number
    net_premium_kurus: number
  }
  versions: VersionView[]
  endorsements: EndorsementView[]
  limit_balances: LimitBalance[]
}

export interface CoverageOnDate {
  policy_id: number
  date: string
  version: VersionView
}

export interface VehicleChangePreview {
  cash_refund_kurus: number
  effective_date: string
  base_version_no: number
  new_version_no: number
  old_annual_premium_kurus: number
  new_annual_premium_kurus: number
  premium_delta_kurus: number
  vehicle_plate: string
  vehicle_group: string
  coverages: CoverageSpec[]
  lines: PremiumLine[]
  calc_detail: CalcDetail
}

export interface CancelPreview {
  cash_refund_kurus: number
  effective_date: string
  reason: string
  method: 'PRO_RATA' | 'SHORT_PERIOD'
  base_version_no: number
  written_premium_kurus: number
  refund_kurus: number
  closes_version: boolean
  calc_detail: CalcDetail
}

export type ClaimStatus = 'REPORTED' | 'TRIAGED' | 'APPRAISAL' | 'ADJUSTMENT' | 'FRAUD_REVIEW' | 'APPROVED' | 'REJECTED' | 'PAID' | 'CLOSED'
export interface ClaimItem {
  id: number
  claim_no: string
  policy_id: number
  policy_no: string
  policy_version_id: number
  coverage_code: string
  loss_date: string
  reported_at: string
  plate: string | null
  description: string
  claimed_amount_kurus: number
  status: ClaimStatus
  row_version: number
}
export interface SettlementStep {
  code: string
  input_kurus: number
  output_kurus: number
  clipped_kurus: number
  description: string
  skipped: boolean
}
export interface SettlementPreview {
  gross_damage_kurus: number
  deductible_kurus: number
  payable_kurus: number
  calc_steps: SettlementStep[]
}
export interface ClaimSettlement extends SettlementPreview {
  id: number
  status: 'APPROVED' | 'PAID'
  paid_at: string | null
  payment_journal_entry_id: number | null
}
export interface ClaimDetail extends ClaimItem {
  fraud?: FraudResult
  tasks?: ClaimTask[]
  allowed_transitions?: ClaimStatus[]
  history: { id: number; from_status: string | null; to_status: string; note: string | null; created_at: string }[]
  appraisals: { id: number; appraiser_id: number; damage_amount_kurus: number; is_total_loss: boolean; notes: string | null; inspected_at: string }[]
  settlement: ClaimSettlement | null
}

export interface BillingPlan {
  policy_id: number
  policy_status: string
  total_kurus: number
  installment_count: number
  open_balance_kurus: number
  daily_rate_ppm: number
  installments: { id: number; seq: number; due_date: string; amount_kurus: number; paid_kurus: number; status: string; overdue_days: number }[]
  interest: { id: number; installment_id: number; accrued_from: string; accrued_to: string; overdue_principal_kurus: number; daily_rate_ppm: number; amount_kurus: number; paid_kurus: number }[]
  payments: { id: number; kind: 'PAYMENT' | 'REFUND'; amount_kurus: number; method: string; paid_at: string; refundable_kurus: number; refund_of_payment_id: number | null; reference: string }[]
  allocations: { id: number; payment_id: number; installment_id: number | null; late_interest_charge_id: number | null; amount_kurus: number }[]
}

export interface BillingRun {
  as_of: string
  accrual_count: number
  interest_kurus: number
  cancelled: number[]
  skipped: { policy_id: number; reason: string }[]
}

export interface TrialBalance {
  accounts: { code: string; name: string; debit_kurus: number; credit_kurus: number; balance_kurus: number }[]
  total_debit_kurus: number
  total_credit_kurus: number
  balanced: boolean
  empty_entries: number
  unbalanced_entries: number
  receivable_reconciliation: { ledger_1200: number; open_installments: number; open_interest: number; matches: boolean }
}

export interface JournalEntry {
  id: number
  entry_date: string
  description: string
  ref_type: string
  ref_id: number
  lines: { id: number; account_code: string; debit_kurus: number; credit_kurus: number }[]
}

export interface FraudResult {
  score: number
  threshold: number
  flagged: boolean
  signals: { rule_code: string; points: number; detail: Record<string, unknown>; explanation: string }[]
}
export interface ClaimTask {
  id: number
  claim_id: number
  claim_no: string
  task_type: 'APPRAISAL' | 'ADJUSTMENT' | 'FRAUD_REVIEW'
  status: 'OPEN' | 'IN_PROGRESS' | 'DONE' | 'CANCELLED'
  assigned_to: { id: number; full_name: string } | null
  created_at: string
  sla_due_at: string
  completed_at: string | null
  breached: boolean
  remaining_seconds: number
  fraud_score?: number
  signal_codes?: string[]
}
export interface TaskWorkload { id: number; full_name: string; role: string; open_tasks: number }
export interface ProductionRow {
  key: string
  label: string
  written_premium_kurus: number
  policy_count: number
  incurred_claims_kurus: number
  loss_ratio_bp: number | null
}
export interface ProductionReport { group_by: string; items: ProductionRow[]; total: ProductionRow }
