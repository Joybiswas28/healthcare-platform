import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

// Typed client for the Go API. Requests go to this app's own /api/v1 proxy,
// which attaches the session cookie's token. Regenerate the types with
// `npm run gen:api` whenever api/openapi.yaml changes.
export const api = createClient<paths>({ baseUrl: "" });

export type Schemas = components["schemas"];
export type User = Schemas["User"];
export type Role = Schemas["Role"];
export type Appointment = Schemas["Appointment"];
export type Chamber = Schemas["Chamber"];
export type Schedule = Schemas["Schedule"];
export type DoctorSummary = Schemas["DoctorSummary"];
export type PublicDoctor = Schemas["PublicDoctor"];
export type Consultation = Schemas["Consultation"];
export type ConsultationStatus = Schemas["ConsultationStatus"];
export type ChatMessage = Schemas["ChatMessage"];
export type Concern = Schemas["Concern"];
export type Offer = Schemas["Offer"];
export type Wallet = Schemas["Wallet"];
export type TokenPackage = Schemas["TokenPackage"];
export type Payment = Schemas["Payment"];
export type LedgerEntry = Schemas["LedgerEntry"];
export type Prescription = Schemas["Prescription"];
export type PrescriptionInput = Schemas["PrescriptionInput"];
export type Medicine = Schemas["Medicine"];
export type PrescriptionVerification = Schemas["PrescriptionVerification"];
export type Hospital = Schemas["Hospital"];
export type HospitalInput = Schemas["HospitalInput"];
export type PublicHospital = Schemas["PublicHospital"];
export type StaffDoctor = Schemas["StaffDoctor"];
export type Photo = Schemas["Photo"];
export type Member = Schemas["Member"];
export type Invitation = Schemas["Invitation"];
export type Pharmacy = Schemas["Pharmacy"];
export type PharmacyInput = Schemas["PharmacyInput"];
export type Review = Schemas["Review"];
export type DoctorReview = Schemas["DoctorReview"];
export type MyDoctorReview = Schemas["MyDoctorReview"];
export type MedicineRequest = Schemas["MedicineRequest"];
export type MedicineRequestInput = Schemas["MedicineRequestInput"];
export type RequestItem = Schemas["Item"];
export type Quote = Schemas["Quote"];
export type QuotedItem = Schemas["QuotedItem"];
export type Order = Schemas["Order"];
export type OrderStatus = Schemas["OrderStatus"];
export type Rider = Schemas["Rider"];
export type Job = Schemas["Job"];

// Turns whatever the API or the network returned into one readable sentence.
export function errorMessage(error: unknown, fallback = "Something went wrong. Please try again."): string {
  if (error && typeof error === "object" && "error" in error && typeof error.error === "string") {
    return error.error;
  }
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}
