export type AssetStatus = 'active' | 'written_off' | 'sold' | 'missing'
export type ProjectStatus = 'tentative' | 'confirmed' | 'in_progress' | 'completed' | 'cancelled'
export type BookingRequestStatus = 'draft' | 'reserved' | 'partially_allocated' | 'out' | 'returned' | 'cancelled'
export type BookingAllocationStatus = 'allocated' | 'checked_out' | 'returned'
export type ServiceStatus = 'open' | 'under_investigation' | 'resolved'
export type ServiceSource = 'monday_report' | 'checkin_damage'
export type UserRole = 'admin' | 'standard'

export interface Product {
  id: number
  legacy_id?: number
  name: string
  category?: string
  manufacturer?: string
  weight_kg?: number
  country_of_origin_code?: string
  is_accessory: boolean
  barcode?: string
  image_url?: string
  description?: string
  active: boolean
  created_at: string
  updated_at: string
}

export interface Asset {
  id: number
  legacy_id?: number
  product_id: number
  asset_number?: string
  serial_number?: string
  is_bulk: boolean
  quantity: number
  location?: string
  purchase_price?: number
  replacement_value?: number
  purchase_date?: string
  status: AssetStatus
  notes?: string
  created_at: string
  updated_at: string
  product_name?: string
  category?: string
  product_image_url?: string
}

export interface ProductListItem extends Product {
  total_units: number
  available_units: number
}

export interface CurrentAllocationInfo {
  allocation_id: number
  project_name: string
  date_out: string
  date_in: string
  status: string
}

export interface ProductAssetItem extends Asset {
  current_allocations: CurrentAllocationInfo[]
}

export interface Project {
  id: number
  name: string
  client?: string
  start_date: string
  end_date: string
  status: ProjectStatus
  carnet_required: boolean
  client_reference?: string
  order_number?: string
  delivery_address?: string
  notes?: string
  created_at: string
  updated_at: string
}

export interface BookingRequest {
  id: number
  project_id: number
  product_id?: number
  placeholder_description?: string
  quantity_requested: number
  date_out: string
  date_in: string
  status: BookingRequestStatus
  shortage_flag: boolean
  sub_hire_notes?: string
  created_at: string
  updated_at: string
  product_name?: string
  category?: string
  project_name?: string
  is_bulk?: boolean
  allocated_count: number
}

export interface BookingAllocation {
  id: number
  booking_request_id: number
  asset_id: number
  status: BookingAllocationStatus
  checked_out_at?: string
  checked_out_by?: number
  inspection_passed?: boolean
  condition_out_notes?: string
  checked_in_at?: string
  checked_in_by?: number
  condition_in_notes?: string
  damage_flag: boolean
  damage_service_record_id?: number
  created_at: string
  updated_at: string
  asset_number?: string
  serial_number?: string
  is_bulk?: boolean
  product_name?: string
  checked_out_by_name?: string
  checked_in_by_name?: string
}

export interface AllocationConflict {
  allocation_id: number
  booking_request_id: number
  project_name: string
  date_out: string
  date_in: string
  status: string
}

export interface AllocationWithConflicts extends BookingAllocation {
  conflicts: AllocationConflict[]
}

export interface ServiceRecord {
  id: number
  asset_id: number
  date_reported: string
  fault_description: string
  status: ServiceStatus
  monday_item_id?: string
  source: ServiceSource
  resolved_date?: string
  resolution_notes?: string
  created_at: string
  updated_at: string
}

export interface CarnetLine {
  product_id: number
  description: string
  quantity: number
  total_weight_kg: number
  total_value: number
  country_of_origin_code?: string
  missing_origin: boolean
}

export interface CarnetView {
  project_id: number
  project_name: string
  lines: CarnetLine[]
  total_weight_kg: number
  total_value: number
  missing_origin: boolean
}

export interface DeliveryNoteLine {
  description: string
  quantity: number
  asset_number?: string
  serial_number?: string
  is_accessory: boolean
}

export interface DeliveryNoteView {
  project: Project
  lines: DeliveryNoteLine[]
  total_weight_kg: number
  total_value: number
}

export interface CurrentUser {
  id: number
  name: string
  email: string
  role: UserRole
}

export interface User {
  id: number
  name: string
  email: string
  role: UserRole
  active: boolean
  created_at: string
  updated_at: string
  has_allocation_history: boolean
}
