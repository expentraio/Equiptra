package models

import "time"

type AssetStatus string

const (
	AssetStatusActive     AssetStatus = "active"
	AssetStatusWrittenOff AssetStatus = "written_off"
	AssetStatusSold       AssetStatus = "sold"
	AssetStatusMissing    AssetStatus = "missing"
)

type ProjectStatus string

const (
	ProjectStatusTentative  ProjectStatus = "tentative"
	ProjectStatusConfirmed  ProjectStatus = "confirmed"
	ProjectStatusInProgress ProjectStatus = "in_progress"
	ProjectStatusCompleted  ProjectStatus = "completed"
	ProjectStatusCancelled  ProjectStatus = "cancelled"
)

type BookingRequestStatus string

const (
	BookingRequestStatusDraft              BookingRequestStatus = "draft"
	BookingRequestStatusReserved           BookingRequestStatus = "reserved"
	BookingRequestStatusPartiallyAllocated BookingRequestStatus = "partially_allocated"
	BookingRequestStatusOut                BookingRequestStatus = "out"
	BookingRequestStatusReturned           BookingRequestStatus = "returned"
	BookingRequestStatusCancelled          BookingRequestStatus = "cancelled"
)

type BookingAllocationStatus string

const (
	BookingAllocationStatusAllocated  BookingAllocationStatus = "allocated"
	BookingAllocationStatusCheckedOut BookingAllocationStatus = "checked_out"
	BookingAllocationStatusReturned   BookingAllocationStatus = "returned"
)

type ServiceStatus string

const (
	ServiceStatusOpen ServiceStatus = "open"
	// ServiceStatusUnderInvestigation is deprecated — left in place for any
	// historic rows, but no longer written by new code. Use
	// ServiceStatusInProgress instead.
	ServiceStatusUnderInvestigation ServiceStatus = "under_investigation"
	ServiceStatusInProgress         ServiceStatus = "in_progress"
	ServiceStatusResolved           ServiceStatus = "resolved"
)

type ServiceSource string

const (
	// ServiceSourceMondayReport is deprecated — the Monday.com relay is
	// retired. Left in place for historic rows; nothing writes it going
	// forward. Use ServiceSourceFieldReport instead.
	ServiceSourceMondayReport  ServiceSource = "monday_report"
	ServiceSourceCheckinDamage ServiceSource = "checkin_damage"
	ServiceSourceFieldReport   ServiceSource = "field_report"
)

type UserRole string

const (
	UserRoleAdmin    UserRole = "admin"
	UserRoleStandard UserRole = "standard"
)

type Product struct {
	ID                  int64     `json:"id"`
	LegacyID            *int      `json:"legacy_id,omitempty"`
	Name                string    `json:"name"`
	Category            *string   `json:"category,omitempty"`
	Manufacturer        *string   `json:"manufacturer,omitempty"`
	WeightKg            *float64  `json:"weight_kg,omitempty"`
	CountryOfOriginCode *string   `json:"country_of_origin_code,omitempty"`
	IsAccessory         bool      `json:"is_accessory"`
	Barcode             *string   `json:"barcode,omitempty"`
	ImageURL            *string   `json:"image_url,omitempty"`
	Description         *string   `json:"description,omitempty"`
	Active              bool      `json:"active"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Asset struct {
	ID               int64       `json:"id"`
	LegacyID         *int        `json:"legacy_id,omitempty"`
	ProductID        int64       `json:"product_id"`
	AssetNumber      *string     `json:"asset_number,omitempty"`
	SerialNumber     *string     `json:"serial_number,omitempty"`
	IsBulk           bool        `json:"is_bulk"`
	Quantity         int         `json:"quantity"`
	Location         *string     `json:"location,omitempty"`
	PurchasePrice    *float64    `json:"purchase_price,omitempty"`
	ReplacementValue *float64    `json:"replacement_value,omitempty"`
	PurchaseDate     *time.Time  `json:"purchase_date,omitempty"`
	Status           AssetStatus `json:"status"`
	Notes            *string     `json:"notes,omitempty"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`

	// Populated on read (joined) endpoints only.
	ProductName     *string `json:"product_name,omitempty"`
	Category        *string `json:"category,omitempty"`
	ProductImageURL *string `json:"product_image_url,omitempty"`
	// HasOpenFault is true if the asset has any open/in_progress
	// service_records entry — excluded from future allocations regardless
	// of Status; see assetHasOpenFault in service_records.go.
	HasOpenFault bool `json:"has_open_fault"`
}

type Project struct {
	ID              int64         `json:"id"`
	Name            string        `json:"name"`
	Client          *string       `json:"client,omitempty"`
	StartDate       time.Time     `json:"start_date"`
	EndDate         time.Time     `json:"end_date"`
	Status          ProjectStatus `json:"status"`
	CarnetRequired  bool          `json:"carnet_required"`
	ClientReference *string       `json:"client_reference,omitempty"`
	OrderNumber     *string       `json:"order_number,omitempty"`
	DeliveryAddress *string       `json:"delivery_address,omitempty"`
	Notes           *string       `json:"notes,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// BookingRequest is the product-level ask against a project — "we need 2x
// Yellobrik OTT1842 for this job." May start as a placeholder (no
// product_id) and get refined later.
type BookingRequest struct {
	ID                     int64                `json:"id"`
	ProjectID              int64                `json:"project_id"`
	ProductID              *int64               `json:"product_id,omitempty"`
	PlaceholderDescription *string              `json:"placeholder_description,omitempty"`
	QuantityRequested      int                  `json:"quantity_requested"`
	DateOut                time.Time            `json:"date_out"`
	DateIn                 time.Time            `json:"date_in"`
	Status                 BookingRequestStatus `json:"status"`
	ShortageFlag           bool                 `json:"shortage_flag"`
	SubHireNotes           *string              `json:"sub_hire_notes,omitempty"`
	CreatedAt              time.Time            `json:"created_at"`
	UpdatedAt              time.Time            `json:"updated_at"`

	// Populated on read (joined) endpoints only.
	ProductName *string `json:"product_name,omitempty"`
	Category    *string `json:"category,omitempty"`
	ProjectName *string `json:"project_name,omitempty"`
	IsBulk      *bool   `json:"is_bulk,omitempty"`
	// Count of active (allocated/checked_out) allocations against this request.
	AllocatedCount int `json:"allocated_count"`
	// Count of allocations against this request regardless of status —
	// includes 'returned', unlike AllocatedCount. This is the real signal
	// for "has allocation history" (e.g. gating project delete), since a
	// fully-returned request still carries checkout/checkin/damage history.
	TotalAllocationCount int `json:"total_allocation_count"`
}

// BookingAllocation is the specific asset assigned once someone pulls a
// physical unit for a booking_request, plus its checkout/check-in lifecycle.
type BookingAllocation struct {
	ID                    int64                   `json:"id"`
	BookingRequestID      int64                   `json:"booking_request_id"`
	AssetID               int64                   `json:"asset_id"`
	Status                BookingAllocationStatus `json:"status"`
	CheckedOutAt          *time.Time              `json:"checked_out_at,omitempty"`
	CheckedOutBy          *int64                  `json:"checked_out_by,omitempty"`
	InspectionPassed      *bool                   `json:"inspection_passed,omitempty"`
	ConditionOutNotes     *string                 `json:"condition_out_notes,omitempty"`
	CheckedInAt           *time.Time              `json:"checked_in_at,omitempty"`
	CheckedInBy           *int64                  `json:"checked_in_by,omitempty"`
	ConditionInNotes      *string                 `json:"condition_in_notes,omitempty"`
	DamageFlag            bool                    `json:"damage_flag"`
	DamageServiceRecordID *int64                  `json:"damage_service_record_id,omitempty"`
	CreatedAt             time.Time               `json:"created_at"`
	UpdatedAt             time.Time               `json:"updated_at"`

	// Populated on read (joined) endpoints only.
	AssetNumber      *string `json:"asset_number,omitempty"`
	SerialNumber     *string `json:"serial_number,omitempty"`
	IsBulk           *bool   `json:"is_bulk,omitempty"`
	ProductName      *string `json:"product_name,omitempty"`
	CheckedOutByName *string `json:"checked_out_by_name,omitempty"`
	CheckedInByName  *string `json:"checked_in_by_name,omitempty"`
}

type ServiceRecord struct {
	ID               int64         `json:"id"`
	AssetID          int64         `json:"asset_id"`
	DateReported     time.Time     `json:"date_reported"`
	FaultDescription string        `json:"fault_description"`
	Status           ServiceStatus `json:"status"`
	MondayItemID     *string       `json:"monday_item_id,omitempty"`
	Source           ServiceSource `json:"source"`
	ResolvedDate     *time.Time    `json:"resolved_date,omitempty"`
	ResolutionNotes  *string       `json:"resolution_notes,omitempty"`
	// ReporterUserID is set for an authenticated (staff) submission;
	// ReporterName/ReporterEmail are set for a freelancer submission with
	// no Equiptra account. Exactly one of the two forms is populated.
	ReporterUserID *int64    `json:"reporter_user_id,omitempty"`
	ReporterName   *string   `json:"reporter_name,omitempty"`
	ReporterEmail  *string   `json:"reporter_email,omitempty"`
	ResolvedBy     *int64    `json:"resolved_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Populated on read (joined) endpoints only.
	AssetNumber      *string `json:"asset_number,omitempty"`
	SerialNumber     *string `json:"serial_number,omitempty"`
	ProductID        int64   `json:"product_id"`
	ProductName      *string `json:"product_name,omitempty"`
	ReporterUserName *string `json:"reporter_user_name,omitempty"`
	ResolvedByName   *string `json:"resolved_by_name,omitempty"`
}

type User struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	Email              string    `json:"email"`
	Role               UserRole  `json:"role"`
	Active             bool      `json:"active"`
	MustChangePassword bool      `json:"must_change_password"`
	PasswordHash       string    `json:"-"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
