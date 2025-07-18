-- Property Booking Management System Database Schema
-- Compatible with PostgreSQL

-- Enable UUID extension for generating unique IDs
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Table for storing users who can manage bookings
CREATE TABLE users (
    user_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(100) NOT NULL,
    role VARCHAR(20) DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Table for storing property information (in case you manage multiple properties)
CREATE TABLE properties (
    property_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    property_name VARCHAR(100) NOT NULL,
    property_address TEXT,
    property_type VARCHAR(50),
    max_guests INTEGER DEFAULT 1,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Table for storing booking information
CREATE TABLE bookings (
    booking_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    property_id UUID NOT NULL REFERENCES properties(property_id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    
    -- Guest primary contact information
    guest_name VARCHAR(100) NOT NULL,
    guest_id_card VARCHAR(50) NOT NULL,
    guest_contact_number VARCHAR(20) NOT NULL,
    guest_email VARCHAR(100),
    
    -- Booking details
    check_in_date DATE NOT NULL,
    check_out_date DATE NOT NULL,
    number_of_guests INTEGER NOT NULL DEFAULT 1,
    total_nights INTEGER GENERATED ALWAYS AS (check_out_date - check_in_date) STORED,
    
    -- Additional information
    booking_notes TEXT,
    special_requests TEXT,
    
    -- Booking status
    booking_status VARCHAR(20) DEFAULT 'confirmed' CHECK (booking_status IN ('pending', 'confirmed', 'cancelled', 'completed')),
    
    -- Financial information (optional)
    booking_amount DECIMAL(10, 2),
    payment_status VARCHAR(20) DEFAULT 'pending' CHECK (payment_status IN ('pending', 'paid', 'partial', 'refunded')),
    
    -- Audit fields
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Constraints
    CONSTRAINT check_dates CHECK (check_out_date > check_in_date),
    CONSTRAINT check_guests CHECK (number_of_guests > 0)
);

-- Table for storing additional guest details (for bookings with multiple guests)
CREATE TABLE booking_guests (
    guest_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    booking_id UUID NOT NULL REFERENCES bookings(booking_id) ON DELETE CASCADE,
    guest_name VARCHAR(100) NOT NULL,
    guest_id_card VARCHAR(50),
    guest_contact_number VARCHAR(20),
    guest_age INTEGER,
    relationship_to_main_guest VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Table for storing booking modifications/history
CREATE TABLE booking_history (
    history_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    booking_id UUID NOT NULL REFERENCES bookings(booking_id) ON DELETE CASCADE,
    modified_by UUID NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
    modification_type VARCHAR(20) NOT NULL CHECK (modification_type IN ('created', 'updated', 'cancelled', 'deleted')),
    old_values JSONB,
    new_values JSONB,
    modification_notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for better performance
CREATE INDEX idx_bookings_property_id ON bookings(property_id);
CREATE INDEX idx_bookings_check_in_date ON bookings(check_in_date);
CREATE INDEX idx_bookings_check_out_date ON bookings(check_out_date);
CREATE INDEX idx_bookings_guest_name ON bookings(guest_name);
CREATE INDEX idx_bookings_status ON bookings(booking_status);
CREATE INDEX idx_bookings_created_by ON bookings(created_by);
CREATE INDEX idx_booking_guests_booking_id ON booking_guests(booking_id);
CREATE INDEX idx_booking_history_booking_id ON booking_history(booking_id);

-- Function to automatically update the updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers to automatically update timestamps
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_properties_updated_at BEFORE UPDATE ON properties FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_bookings_updated_at BEFORE UPDATE ON bookings FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to prevent overlapping bookings for the same property
CREATE OR REPLACE FUNCTION check_booking_overlap()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM bookings 
        WHERE property_id = NEW.property_id 
        AND booking_status IN ('confirmed', 'pending')
        AND booking_id != COALESCE(NEW.booking_id, '00000000-0000-0000-0000-000000000000'::UUID)
        AND (
            (NEW.check_in_date >= check_in_date AND NEW.check_in_date < check_out_date) OR
            (NEW.check_out_date > check_in_date AND NEW.check_out_date <= check_out_date) OR
            (NEW.check_in_date <= check_in_date AND NEW.check_out_date >= check_out_date)
        )
    ) THEN
        RAISE EXCEPTION 'Booking dates overlap with existing booking for this property';
    END IF;
    
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to prevent overlapping bookings
CREATE TRIGGER prevent_booking_overlap 
    BEFORE INSERT OR UPDATE ON bookings 
    FOR EACH ROW EXECUTE FUNCTION check_booking_overlap();

-- Function to automatically log booking changes
CREATE OR REPLACE FUNCTION log_booking_changes()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO booking_history (booking_id, modified_by, modification_type, new_values)
        VALUES (NEW.booking_id, NEW.created_by, 'created', to_jsonb(NEW));
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO booking_history (booking_id, modified_by, modification_type, old_values, new_values)
        VALUES (NEW.booking_id, NEW.created_by, 'updated', to_jsonb(OLD), to_jsonb(NEW));
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        INSERT INTO booking_history (booking_id, modified_by, modification_type, old_values)
        VALUES (OLD.booking_id, OLD.created_by, 'deleted', to_jsonb(OLD));
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ language 'plpgsql';

-- Trigger to automatically log booking changes
CREATE TRIGGER log_booking_changes_trigger 
    AFTER INSERT OR UPDATE OR DELETE ON bookings 
    FOR EACH ROW EXECUTE FUNCTION log_booking_changes();

-- Sample data insertion (optional)
-- Insert a default property
INSERT INTO properties (property_name, property_address, property_type, max_guests, description)
VALUES ('My Property', '123 Main Street, City, Country', 'Apartment', 4, 'Beautiful apartment for short-term stays');

-- Insert sample users (you'll need to hash passwords properly in your application)
INSERT INTO users (username, email, password_hash, full_name, role)
VALUES 
    ('admin', 'admin@example.com', '$2b$12$example_hash_here', 'Administrator', 'admin'),
    ('manager1', 'manager1@example.com', '$2b$12$example_hash_here', 'Property Manager 1', 'user'),
    ('manager2', 'manager2@example.com', '$2b$12$example_hash_here', 'Property Manager 2', 'user');

-- Useful queries for your application:

-- 1. Get all bookings for a specific date range
-- SELECT * FROM bookings 
-- WHERE check_in_date <= '2024-12-31' AND check_out_date >= '2024-01-01'
-- ORDER BY check_in_date;

-- 2. Get availability for a property in a date range
-- SELECT date_trunc('day', dd) as available_date
-- FROM generate_series('2024-01-01'::date, '2024-12-31'::date, '1 day'::interval) dd
-- WHERE NOT EXISTS (
--     SELECT 1 FROM bookings 
--     WHERE property_id = 'your-property-id' 
--     AND booking_status IN ('confirmed', 'pending')
--     AND dd >= check_in_date AND dd < check_out_date
-- );

-- 3. Get booking details with guest information
-- SELECT b.*, bg.guest_name as additional_guest_name, bg.guest_id_card as additional_guest_id
-- FROM bookings b
-- LEFT JOIN booking_guests bg ON b.booking_id = bg.booking_id
-- WHERE b.booking_id = 'your-booking-id';

-- 4. Get booking history for audit purposes
-- SELECT bh.*, u.full_name as modified_by_name
-- FROM booking_history bh
-- JOIN users u ON bh.modified_by = u.user_id
-- WHERE bh.booking_id = 'your-booking-id'
-- ORDER BY bh.created_at DESC;