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
