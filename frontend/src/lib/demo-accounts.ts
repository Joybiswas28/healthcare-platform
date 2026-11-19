// Accounts that exist in the local development database only. A real server
// starts empty, so these never work there; the login page shows them only
// under `npm run dev`.
export const SHOW_DEMO_ACCOUNTS = process.env.NODE_ENV !== "production";

// The condition is fixed at build time, so in a production build the list is
// an empty array and the passwords are not shipped to browsers at all.
export const DEMO_ACCOUNTS = SHOW_DEMO_ACCOUNTS
  ? [
      { role: "Patient", name: "Karim Uddin", phone: "01866666666", password: "patient123" },
      { role: "Doctor", name: "Dr Ayesha Siddiqua", phone: "01755555555", password: "doctor123" },
      { role: "Hospital admin", name: "Square Hospitals", phone: "01600000001", password: "square123" },
      { role: "Pharmacy", name: "Lazz Pharma", phone: "01500000001", password: "secret123" },
      { role: "Delivery rider", name: "Rashed", phone: "01500000002", password: "secret123" },
      { role: "Admin", name: "Platform admin", phone: "01900000000", password: "admin-pass-12345" },
    ]
  : [];
