package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

// Database connection
var db *sql.DB

// Models
type User struct {
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Property struct {
	PropertyID      uuid.UUID `json:"property_id"`
	PropertyName    string    `json:"property_name"`
	PropertyAddress string    `json:"property_address"`
	PropertyType    string    `json:"property_type"`
	MaxGuests       int       `json:"max_guests"`
	Description     string    `json:"description"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Booking struct {
	BookingID          uuid.UUID `json:"booking_id"`
	PropertyID         uuid.UUID `json:"property_id"`
	CreatedBy          uuid.UUID `json:"created_by"`
	GuestName          string    `json:"guest_name"`
	GuestIDCard        string    `json:"guest_id_card"`
	GuestContactNumber string    `json:"guest_contact_number"`
	GuestEmail         *string   `json:"guest_email,omitempty"`
	CheckInDate        time.Time `json:"check_in_date"`
	CheckOutDate       time.Time `json:"check_out_date"`
	NumberOfGuests     int       `json:"number_of_guests"`
	TotalNights        int       `json:"total_nights"`
	BookingNotes       *string   `json:"booking_notes,omitempty"`
	SpecialRequests    *string   `json:"special_requests,omitempty"`
	BookingStatus      string    `json:"booking_status"`
	BookingAmount      *float64  `json:"booking_amount,omitempty"`
	PaymentStatus      string    `json:"payment_status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	AdditionalGuests   []Guest   `json:"additional_guests,omitempty"`
}

type Guest struct {
	GuestID                 uuid.UUID `json:"guest_id"`
	BookingID               uuid.UUID `json:"booking_id"`
	GuestName               string    `json:"guest_name"`
	GuestIDCard             *string   `json:"guest_id_card,omitempty"`
	GuestContactNumber      *string   `json:"guest_contact_number,omitempty"`
	GuestAge                *int      `json:"guest_age,omitempty"`
	RelationshipToMainGuest *string   `json:"relationship_to_main_guest,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
}

type CalendarDay struct {
	Date      time.Time  `json:"date"`
	IsBooked  bool       `json:"is_booked"`
	BookingID *uuid.UUID `json:"booking_id,omitempty"`
}

type MonthCalendar struct {
	Year  int           `json:"year"`
	Month int           `json:"month"`
	Days  []CalendarDay `json:"days"`
}

// Request/Response DTOs
type CreateBookingRequest struct {
	PropertyID         uuid.UUID            `json:"property_id"`
	GuestName          string               `json:"guest_name"`
	GuestIDCard        string               `json:"guest_id_card"`
	GuestContactNumber string               `json:"guest_contact_number"`
	GuestEmail         *string              `json:"guest_email,omitempty"`
	CheckInDate        string               `json:"check_in_date"`  // "2024-01-15" format
	CheckOutDate       string               `json:"check_out_date"` // "2024-01-20" format
	NumberOfGuests     int                  `json:"number_of_guests"`
	BookingNotes       *string              `json:"booking_notes,omitempty"`
	SpecialRequests    *string              `json:"special_requests,omitempty"`
	BookingAmount      *float64             `json:"booking_amount,omitempty"`
	AdditionalGuests   []CreateGuestRequest `json:"additional_guests,omitempty"`
}

type CreateGuestRequest struct {
	GuestName               string  `json:"guest_name"`
	GuestIDCard             *string `json:"guest_id_card,omitempty"`
	GuestContactNumber      *string `json:"guest_contact_number,omitempty"`
	GuestAge                *int    `json:"guest_age,omitempty"`
	RelationshipToMainGuest *string `json:"relationship_to_main_guest,omitempty"`
}

type UpdateBookingRequest struct {
	GuestName          *string  `json:"guest_name,omitempty"`
	GuestIDCard        *string  `json:"guest_id_card,omitempty"`
	GuestContactNumber *string  `json:"guest_contact_number,omitempty"`
	GuestEmail         *string  `json:"guest_email,omitempty"`
	CheckInDate        *string  `json:"check_in_date,omitempty"`
	CheckOutDate       *string  `json:"check_out_date,omitempty"`
	NumberOfGuests     *int     `json:"number_of_guests,omitempty"`
	BookingNotes       *string  `json:"booking_notes,omitempty"`
	SpecialRequests    *string  `json:"special_requests,omitempty"`
	BookingAmount      *float64 `json:"booking_amount,omitempty"`
	BookingStatus      *string  `json:"booking_status,omitempty"`
	PaymentStatus      *string  `json:"payment_status,omitempty"`
}

// Service layer
type BookingService struct {
	db *sql.DB
}

func NewBookingService(database *sql.DB) *BookingService {
	return &BookingService{db: database}
}

// 1. Loading a calendar by month and see which dates have been booked
func (s *BookingService) GetMonthCalendar(propertyID uuid.UUID, year, month int) (*MonthCalendar, error) {
	// Get first and last day of the month
	firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstDay.AddDate(0, 1, -1)

	// Get all bookings for this property in this month
	query := `
		SELECT booking_id, check_in_date, check_out_date 
		FROM bookings 
		WHERE property_id = $1 
		AND booking_status IN ('confirmed', 'pending')
		AND (check_in_date <= $2 AND check_out_date > $3)
	`

	rows, err := s.db.Query(query, propertyID, lastDay, firstDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Create a map to track booked dates
	bookedDates := make(map[string]uuid.UUID)

	for rows.Next() {
		var bookingID uuid.UUID
		var checkIn, checkOut time.Time

		if err := rows.Scan(&bookingID, &checkIn, &checkOut); err != nil {
			return nil, err
		}

		// Mark all dates in the booking range as booked
		for d := checkIn; d.Before(checkOut); d = d.AddDate(0, 0, 1) {
			if d.Year() == year && int(d.Month()) == month {
				bookedDates[d.Format("2006-01-02")] = bookingID
			}
		}
	}

	// Build calendar days
	var days []CalendarDay
	for d := firstDay; !d.After(lastDay); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		bookingID, isBooked := bookedDates[dateStr]

		day := CalendarDay{
			Date:     d,
			IsBooked: isBooked,
		}

		if isBooked {
			day.BookingID = &bookingID
		}

		days = append(days, day)
	}

	return &MonthCalendar{
		Year:  year,
		Month: month,
		Days:  days,
	}, nil
}

// 2. Create a booking from a given date to checkout date
func (s *BookingService) CreateBooking(userID uuid.UUID, req *CreateBookingRequest) (*Booking, error) {
	// Parse dates
	checkInDate, err := time.Parse("2006-01-02", req.CheckInDate)
	if err != nil {
		return nil, fmt.Errorf("invalid check-in date format: %v", err)
	}

	checkOutDate, err := time.Parse("2006-01-02", req.CheckOutDate)
	if err != nil {
		return nil, fmt.Errorf("invalid check-out date format: %v", err)
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Insert booking
	bookingID := uuid.New()
	query := `
		INSERT INTO bookings (
			booking_id, property_id, created_by, guest_name, guest_id_card, 
			guest_contact_number, guest_email, check_in_date, check_out_date, 
			number_of_guests, booking_notes, special_requests, booking_amount
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err = tx.Exec(query, bookingID, req.PropertyID, userID, req.GuestName,
		req.GuestIDCard, req.GuestContactNumber, req.GuestEmail, checkInDate,
		checkOutDate, req.NumberOfGuests, req.BookingNotes, req.SpecialRequests,
		req.BookingAmount)
	if err != nil {
		return nil, err
	}

	// Insert additional guests
	for _, guest := range req.AdditionalGuests {
		guestID := uuid.New()
		guestQuery := `
			INSERT INTO booking_guests (
				guest_id, booking_id, guest_name, guest_id_card, 
				guest_contact_number, guest_age, relationship_to_main_guest
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
		`
		_, err = tx.Exec(guestQuery, guestID, bookingID, guest.GuestName,
			guest.GuestIDCard, guest.GuestContactNumber, guest.GuestAge,
			guest.RelationshipToMainGuest)
		if err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	// Return the created booking
	return s.GetBookingByID(bookingID)
}

// 3. Get upcoming bookings up to a selected date
func (s *BookingService) GetUpcomingBookings(propertyID uuid.UUID, upToDate time.Time) ([]Booking, error) {
	query := `
		SELECT booking_id, property_id, created_by, guest_name, guest_id_card,
			guest_contact_number, guest_email, check_in_date, check_out_date,
			number_of_guests, total_nights, booking_notes, special_requests,
			booking_status, booking_amount, payment_status, created_at, updated_at
		FROM bookings
		WHERE property_id = $1
		AND check_in_date >= CURRENT_DATE
		AND check_in_date <= $2
		AND booking_status IN ('confirmed', 'pending')
		ORDER BY check_in_date ASC
	`

	return s.queryBookings(query, propertyID, upToDate)
}

// 4. Get previous bookings up to a selected date
func (s *BookingService) GetPreviousBookings(propertyID uuid.UUID, backToDate time.Time) ([]Booking, error) {
	query := `
		SELECT booking_id, property_id, created_by, guest_name, guest_id_card,
			guest_contact_number, guest_email, check_in_date, check_out_date,
			number_of_guests, total_nights, booking_notes, special_requests,
			booking_status, booking_amount, payment_status, created_at, updated_at
		FROM bookings
		WHERE property_id = $1
		AND check_out_date < CURRENT_DATE
		AND check_out_date >= $2
		ORDER BY check_out_date DESC
	`

	return s.queryBookings(query, propertyID, backToDate)
}

// 5. Cancel an upcoming booking
func (s *BookingService) CancelBooking(bookingID uuid.UUID, userID uuid.UUID) error {
	query := `
		UPDATE bookings 
		SET booking_status = 'cancelled', updated_at = CURRENT_TIMESTAMP
		WHERE booking_id = $1 
		AND check_in_date >= CURRENT_DATE
		AND booking_status IN ('confirmed', 'pending')
	`

	result, err := s.db.Exec(query, bookingID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("booking not found or cannot be cancelled")
	}

	return nil
}

// 6. Edit a booking
func (s *BookingService) UpdateBooking(bookingID uuid.UUID, userID uuid.UUID, req *UpdateBookingRequest) (*Booking, error) {
	// Build dynamic update query
	setParts := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.GuestName != nil {
		setParts = append(setParts, fmt.Sprintf("guest_name = $%d", argIndex))
		args = append(args, *req.GuestName)
		argIndex++
	}

	if req.GuestIDCard != nil {
		setParts = append(setParts, fmt.Sprintf("guest_id_card = $%d", argIndex))
		args = append(args, *req.GuestIDCard)
		argIndex++
	}

	if req.GuestContactNumber != nil {
		setParts = append(setParts, fmt.Sprintf("guest_contact_number = $%d", argIndex))
		args = append(args, *req.GuestContactNumber)
		argIndex++
	}

	if req.GuestEmail != nil {
		setParts = append(setParts, fmt.Sprintf("guest_email = $%d", argIndex))
		args = append(args, *req.GuestEmail)
		argIndex++
	}

	if req.CheckInDate != nil {
		checkInDate, err := time.Parse("2006-01-02", *req.CheckInDate)
		if err != nil {
			return nil, fmt.Errorf("invalid check-in date format: %v", err)
		}
		setParts = append(setParts, fmt.Sprintf("check_in_date = $%d", argIndex))
		args = append(args, checkInDate)
		argIndex++
	}

	if req.CheckOutDate != nil {
		checkOutDate, err := time.Parse("2006-01-02", *req.CheckOutDate)
		if err != nil {
			return nil, fmt.Errorf("invalid check-out date format: %v", err)
		}
		setParts = append(setParts, fmt.Sprintf("check_out_date = $%d", argIndex))
		args = append(args, checkOutDate)
		argIndex++
	}

	if req.NumberOfGuests != nil {
		setParts = append(setParts, fmt.Sprintf("number_of_guests = $%d", argIndex))
		args = append(args, *req.NumberOfGuests)
		argIndex++
	}

	if req.BookingNotes != nil {
		setParts = append(setParts, fmt.Sprintf("booking_notes = $%d", argIndex))
		args = append(args, *req.BookingNotes)
		argIndex++
	}

	if req.SpecialRequests != nil {
		setParts = append(setParts, fmt.Sprintf("special_requests = $%d", argIndex))
		args = append(args, *req.SpecialRequests)
		argIndex++
	}

	if req.BookingAmount != nil {
		setParts = append(setParts, fmt.Sprintf("booking_amount = $%d", argIndex))
		args = append(args, *req.BookingAmount)
		argIndex++
	}

	if req.BookingStatus != nil {
		setParts = append(setParts, fmt.Sprintf("booking_status = $%d", argIndex))
		args = append(args, *req.BookingStatus)
		argIndex++
	}

	if req.PaymentStatus != nil {
		setParts = append(setParts, fmt.Sprintf("payment_status = $%d", argIndex))
		args = append(args, *req.PaymentStatus)
		argIndex++
	}

	if len(setParts) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Add updated_at
	setParts = append(setParts, fmt.Sprintf("updated_at = CURRENT_TIMESTAMP"))

	// Add WHERE clause parameters
	args = append(args, bookingID)
	whereClause := fmt.Sprintf("WHERE booking_id = $%d", argIndex)

	query := fmt.Sprintf("UPDATE bookings SET %s %s", strings.Join(setParts, ", "), whereClause)

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("booking not found")
	}

	return s.GetBookingByID(bookingID)
}

// 7. Search for bookings by guest name
func (s *BookingService) SearchBookingsByGuestName(propertyID uuid.UUID, guestName string) ([]Booking, error) {
	query := `
		SELECT booking_id, property_id, created_by, guest_name, guest_id_card,
			guest_contact_number, guest_email, check_in_date, check_out_date,
			number_of_guests, total_nights, booking_notes, special_requests,
			booking_status, booking_amount, payment_status, created_at, updated_at
		FROM bookings
		WHERE property_id = $1
		AND LOWER(guest_name) LIKE LOWER($2)
		ORDER BY check_in_date DESC
	`

	searchPattern := "%" + guestName + "%"
	return s.queryBookings(query, propertyID, searchPattern)
}

// Helper methods
func (s *BookingService) GetBookingByID(bookingID uuid.UUID) (*Booking, error) {
	query := `
		SELECT booking_id, property_id, created_by, guest_name, guest_id_card,
			guest_contact_number, guest_email, check_in_date, check_out_date,
			number_of_guests, total_nights, booking_notes, special_requests,
			booking_status, booking_amount, payment_status, created_at, updated_at
		FROM bookings
		WHERE booking_id = $1
	`

	bookings, err := s.queryBookings(query, bookingID)
	if err != nil {
		return nil, err
	}

	if len(bookings) == 0 {
		return nil, fmt.Errorf("booking not found")
	}

	return &bookings[0], nil
}

func (s *BookingService) queryBookings(query string, args ...interface{}) ([]Booking, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking

	for rows.Next() {
		var booking Booking

		err := rows.Scan(
			&booking.BookingID, &booking.PropertyID, &booking.CreatedBy,
			&booking.GuestName, &booking.GuestIDCard, &booking.GuestContactNumber,
			&booking.GuestEmail, &booking.CheckInDate, &booking.CheckOutDate,
			&booking.NumberOfGuests, &booking.TotalNights, &booking.BookingNotes,
			&booking.SpecialRequests, &booking.BookingStatus, &booking.BookingAmount,
			&booking.PaymentStatus, &booking.CreatedAt, &booking.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		// Load additional guests
		guests, err := s.getAdditionalGuests(booking.BookingID)
		if err != nil {
			return nil, err
		}
		booking.AdditionalGuests = guests

		bookings = append(bookings, booking)
	}

	return bookings, nil
}

func (s *BookingService) getAdditionalGuests(bookingID uuid.UUID) ([]Guest, error) {
	query := `
		SELECT guest_id, booking_id, guest_name, guest_id_card, guest_contact_number,
			guest_age, relationship_to_main_guest, created_at
		FROM booking_guests
		WHERE booking_id = $1
	`

	rows, err := s.db.Query(query, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guests []Guest

	for rows.Next() {
		var guest Guest
		err := rows.Scan(
			&guest.GuestID, &guest.BookingID, &guest.GuestName,
			&guest.GuestIDCard, &guest.GuestContactNumber, &guest.GuestAge,
			&guest.RelationshipToMainGuest, &guest.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		guests = append(guests, guest)
	}

	return guests, nil
}

// HTTP Handlers
func (s *BookingService) GetMonthCalendarHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	propertyIDStr := vars["propertyId"]
	yearStr := vars["year"]
	monthStr := vars["month"]

	propertyID, err := uuid.Parse(propertyIDStr)
	if err != nil {
		http.Error(w, "Invalid property ID", http.StatusBadRequest)
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		http.Error(w, "Invalid month", http.StatusBadRequest)
		return
	}

	calendar, err := s.GetMonthCalendar(propertyID, year, month)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(calendar)
}

func (s *BookingService) CreateBookingHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// In a real application, you would extract userID from JWT token or session
	userID := uuid.New() // Mock user ID

	booking, err := s.CreateBooking(userID, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(booking)
}

func (s *BookingService) GetUpcomingBookingsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	propertyIDStr := vars["propertyId"]
	upToDateStr := r.URL.Query().Get("up_to_date")

	propertyID, err := uuid.Parse(propertyIDStr)
	if err != nil {
		http.Error(w, "Invalid property ID", http.StatusBadRequest)
		return
	}

	upToDate := time.Now().AddDate(0, 3, 0) // Default: 3 months from now
	if upToDateStr != "" {
		upToDate, err = time.Parse("2006-01-02", upToDateStr)
		if err != nil {
			http.Error(w, "Invalid up_to_date format", http.StatusBadRequest)
			return
		}
	}

	bookings, err := s.GetUpcomingBookings(propertyID, upToDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookings)
}

func (s *BookingService) GetPreviousBookingsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	propertyIDStr := vars["propertyId"]
	backToDateStr := r.URL.Query().Get("back_to_date")

	propertyID, err := uuid.Parse(propertyIDStr)
	if err != nil {
		http.Error(w, "Invalid property ID", http.StatusBadRequest)
		return
	}

	backToDate := time.Now().AddDate(0, -3, 0) // Default: 3 months ago
	if backToDateStr != "" {
		backToDate, err = time.Parse("2006-01-02", backToDateStr)
		if err != nil {
			http.Error(w, "Invalid back_to_date format", http.StatusBadRequest)
			return
		}
	}

	bookings, err := s.GetPreviousBookings(propertyID, backToDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookings)
}

func (s *BookingService) CancelBookingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bookingIDStr := vars["bookingId"]

	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		http.Error(w, "Invalid booking ID", http.StatusBadRequest)
		return
	}

	// In a real application, you would extract userID from JWT token or session
	userID := uuid.New() // Mock user ID

	err = s.CancelBooking(bookingID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *BookingService) UpdateBookingHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bookingIDStr := vars["bookingId"]

	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		http.Error(w, "Invalid booking ID", http.StatusBadRequest)
		return
	}

	var req UpdateBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// In a real application, you would extract userID from JWT token or session
	userID := uuid.New() // Mock user ID

	booking, err := s.UpdateBooking(bookingID, userID, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(booking)
}

func (s *BookingService) SearchBookingsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	propertyIDStr := vars["propertyId"]
	guestName := r.URL.Query().Get("guest_name")

	propertyID, err := uuid.Parse(propertyIDStr)
	if err != nil {
		http.Error(w, "Invalid property ID", http.StatusBadRequest)
		return
	}

	if guestName == "" {
		http.Error(w, "guest_name parameter is required", http.StatusBadRequest)
		return
	}

	bookings, err := s.SearchBookingsByGuestName(propertyID, guestName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookings)
}

// Database initialization
func initDB() error {
	var err error

	config := LoadConfig()

	// Database connection string - adjust as needed
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.DBHost, config.DBPort, config.DBUser, config.DBPassword, config.DBName, config.DBSSLMode)

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Test the connection
	if err = db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	log.Println("Database connection established")
	return nil
}

// Setup routes
func setupRoutes(service *BookingService) *mux.Router {
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api/v1").Subrouter()

	// 1. Get calendar for a specific month
	api.HandleFunc("/properties/{propertyId}/calendar/{year}/{month}", service.GetMonthCalendarHandler).Methods("GET")

	// 2. Create a new booking
	api.HandleFunc("/bookings", service.CreateBookingHandler).Methods("POST")

	// 3. Get upcoming bookings
	api.HandleFunc("/properties/{propertyId}/bookings/upcoming", service.GetUpcomingBookingsHandler).Methods("GET")

	// 4. Get previous bookings
	api.HandleFunc("/properties/{propertyId}/bookings/previous", service.GetPreviousBookingsHandler).Methods("GET")

	// 5. Cancel a booking
	api.HandleFunc("/bookings/{bookingId}/cancel", service.CancelBookingHandler).Methods("PUT")

	// 6. Update a booking
	api.HandleFunc("/bookings/{bookingId}", service.UpdateBookingHandler).Methods("PUT")

	// 7. Search bookings by guest name
	api.HandleFunc("/properties/{propertyId}/bookings/search", service.SearchBookingsHandler).Methods("GET")

	// Additional utility endpoints
	api.HandleFunc("/bookings/{bookingId}", service.GetBookingByIDHandler).Methods("GET")
	api.HandleFunc("/properties", service.GetPropertiesHandler).Methods("GET")

	return r
}

// Additional handlers
func (s *BookingService) GetBookingByIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bookingIDStr := vars["bookingId"]

	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		http.Error(w, "Invalid booking ID", http.StatusBadRequest)
		return
	}

	booking, err := s.GetBookingByID(bookingID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(booking)
}

func (s *BookingService) GetPropertiesHandler(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT property_id, property_name, property_address, property_type, 
			max_guests, description, created_at, updated_at
		FROM properties
		ORDER BY property_name
	`

	rows, err := s.db.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var properties []Property

	for rows.Next() {
		var property Property
		err := rows.Scan(
			&property.PropertyID, &property.PropertyName, &property.PropertyAddress,
			&property.PropertyType, &property.MaxGuests, &property.Description,
			&property.CreatedAt, &property.UpdatedAt,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		properties = append(properties, property)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(properties)
}

// CORS middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Logging middleware
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.RequestURI, time.Since(start))
	})
}

// Main function
func main() {
	// Initialize database
	if err := initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Create service
	service := NewBookingService(db)

	// Setup routes
	router := setupRoutes(service)

	// Add middleware
	router.Use(corsMiddleware)
	router.Use(loggingMiddleware)

	// Start server
	port := ":8080"
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(port, router))
}

/*
API Usage Examples:

1. Get calendar for January 2024:
GET /api/v1/properties/{propertyId}/calendar/2024/1

2. Create a new booking:
POST /api/v1/bookings
{
  "property_id": "uuid-here",
  "guest_name": "John Doe",
  "guest_id_card": "ID123456",
  "guest_contact_number": "+1234567890",
  "guest_email": "john@example.com",
  "check_in_date": "2024-01-15",
  "check_out_date": "2024-01-20",
  "number_of_guests": 2,
  "booking_notes": "Anniversary celebration",
  "booking_amount": 500.00,
  "additional_guests": [
    {
      "guest_name": "Jane Doe",
      "guest_id_card": "ID123457",
      "guest_contact_number": "+1234567891",
      "guest_age": 30,
      "relationship_to_main_guest": "spouse"
    }
  ]
}

3. Get upcoming bookings:
GET /api/v1/properties/{propertyId}/bookings/upcoming?up_to_date=2024-06-30

4. Get previous bookings:
GET /api/v1/properties/{propertyId}/bookings/previous?back_to_date=2024-01-01

5. Cancel a booking:
PUT /api/v1/bookings/{bookingId}/cancel

6. Update a booking:
PUT /api/v1/bookings/{bookingId}
{
  "guest_name": "John Smith",
  "number_of_guests": 3,
  "booking_notes": "Updated notes"
}

7. Search bookings by guest name:
GET /api/v1/properties/{propertyId}/bookings/search?guest_name=John

8. Get a specific booking:
GET /api/v1/bookings/{bookingId}

9. Get all properties:
GET /api/v1/properties

Dependencies (go.mod):
module booking-service

go 1.21

require (
    github.com/google/uuid v1.3.0
    github.com/gorilla/mux v1.8.0
    github.com/lib/pq v1.10.9
)

To run the service:
1. Install dependencies: go mod tidy
2. Update database connection string in initDB()
3. Run the database schema script first
4. Start the service: go run main.go
*/
