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
  effective_date: string
  reason: string
  method: 'PRO_RATA' | 'SHORT_PERIOD'
  base_version_no: number
  written_premium_kurus: number
  refund_kurus: number
  closes_version: boolean
  calc_detail: CalcDetail
}
