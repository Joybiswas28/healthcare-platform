// Sylhet hospital and ambulance contacts, collected from public sources on
// 2026-11-19. These are NOT hospitals registered on this platform; it is a
// reference list. Every number keeps the page it came from so it can be
// re-checked. Numbers change: re-verify before relying on this list and update
// CHECKED_ON when you do.
//
// "official": printed on the provider's own website, or on the UK government's
//             list of medical facilities in Bangladesh (updated September 2026).
// "directory": found only on third-party directories, possibly out of date.

export const CHECKED_ON = "2026-11-19";

export type Confidence = "official" | "directory";

export type Phone = { label: string; number: string };

export type DirectoryEntry = {
  name: string;
  kind: "Public" | "Private";
  address: string;
  phones: Phone[];
  ambulance?: Phone[];
  website?: string;
  note?: string;
  confidence: Confidence;
  sources: { label: string; url: string }[];
};

const GOV_UK = {
  label: "GOV.UK list of medical facilities (Sept 2026)",
  url: "https://www.gov.uk/government/publications/bangladesh-list-of-medical-facilities/bangladesh-doctors-and-medical-facilities",
};

export const NATIONAL_EMERGENCY: Phone[] = [
  { label: "National emergency (police, fire, ambulance)", number: "999" },
  { label: "Shastho Batayon health helpline (DGHS)", number: "16263" },
];

export const HOSPITALS: DirectoryEntry[] = [
  {
    name: "Sylhet MAG Osmani Medical College Hospital",
    kind: "Public",
    address: "Medical Road, Kajolshah, Sylhet-3100",
    phones: [{ label: "Emergency contact", number: "02996631213" }],
    website: "https://www.somch.gov.bd/",
    note: "Government hospital, all specialities. Runs an emergency ambulance service (no separate number published).",
    confidence: "official",
    sources: [{ label: "somch.gov.bd", url: "https://www.somch.gov.bd/" }, GOV_UK],
  },
  {
    name: "Mount Adora Hospital, Akhalia",
    kind: "Private",
    address: "Akhalia, Sylhet–Sunamganj Road, Sylhet-3100",
    phones: [{ label: "Hotline", number: "+8809610848484" }],
    ambulance: [
      { label: "Emergency & ambulance dispatch (24/7)", number: "+8809610848484" },
      { label: "Ambulance service", number: "+8801711407457" },
    ],
    website: "https://mountadora.com/",
    confidence: "official",
    sources: [{ label: "mountadora.com", url: "https://mountadora.com/" }, GOV_UK],
  },
  {
    name: "Mount Adora Hospital, Nayasarak",
    kind: "Private",
    address: "Mirboxtula, Nayasarak, Sylhet-3100",
    phones: [{ label: "Hotline", number: "+8809610858585" }],
    ambulance: [
      { label: "Emergency & ambulance dispatch (24/7)", number: "+8809610858585" },
      { label: "Ambulance service", number: "+8801334912408" },
    ],
    website: "https://mountadora.com/",
    confidence: "official",
    sources: [{ label: "mountadora.com", url: "https://mountadora.com/" }],
  },
  {
    name: "Ibn Sina Hospital Sylhet",
    kind: "Private",
    address: "Subhanighat Point, Sylhet",
    phones: [
      { label: "Hotline / emergency", number: "+8809610009640" },
      { label: "PABX (lines 10–19)", number: "+8802996640010" },
    ],
    website: "https://ibnsinahospitalsylhet.com.bd/",
    confidence: "official",
    sources: [{ label: "ibnsinahospitalsylhet.com.bd", url: "https://ibnsinahospitalsylhet.com.bd/contact-us.php" }],
  },
  {
    name: "Jalalabad Ragib-Rabeya Medical College & Hospital",
    kind: "Private",
    address: "Ragib-Rabeya Medical College Road, Pathantula, Sylhet",
    phones: [{ label: "24/7 emergency hotline (main office)", number: "01779900016" }],
    website: "https://www.jrrmc.edu.bd/",
    confidence: "official",
    sources: [{ label: "jrrmc.edu.bd", url: "https://www.jrrmc.edu.bd/common/contact" }, GOV_UK],
  },
  {
    name: "Parkview Medical College Hospital",
    kind: "Private",
    address: "VIP Road, Taltola, Sylhet-3100",
    phones: [
      { label: "Hospital", number: "01746094823" },
      { label: "Hospital", number: "01849868959" },
    ],
    website: "https://pmch.edu.bd/",
    confidence: "official",
    sources: [{ label: "pmch.edu.bd", url: "https://pmch.edu.bd/" }, GOV_UK],
  },
  {
    name: "Sylhet Women's Medical College Hospital",
    kind: "Private",
    address: "Mirboxtula, Sylhet",
    phones: [
      { label: "Hospital mobile", number: "+8801730639024" },
      { label: "Hospital", number: "+8802997706866" },
    ],
    website: "https://swmc.edu.bd/",
    note: "The site's home and contact pages list different landlines; the mobile number appears on the home page.",
    confidence: "official",
    sources: [{ label: "swmc.edu.bd", url: "https://swmc.edu.bd/contact" }],
  },
  {
    name: "Oasis Hospital",
    kind: "Private",
    address: "Subhanighat, Bishwa Road, Sylhet-3100",
    phones: [
      { label: "Phone", number: "01763990055" },
      { label: "Phone", number: "01763990044" },
    ],
    website: "https://www.oasishospitalbd.com/",
    note: "Read from the hospital's home page; its contact page was down when checked, so this couldn't be confirmed there.",
    confidence: "directory",
    sources: [{ label: "oasishospitalbd.com", url: "https://www.oasishospitalbd.com/" }],
  },
  {
    name: "North East Medical College Hospital",
    kind: "Private",
    address: "Chondipul, South Surma, Sylhet-3100",
    phones: [{ label: "Mobile", number: "01715944733" }],
    website: "https://www.nemc.edu.bd/",
    note: "The hospital's website did not show contact details when checked.",
    confidence: "directory",
    sources: [{ label: "bd-directory.com", url: "https://www.bd-directory.com/North_East_Medical_College_Sylhet.html" }],
  },
];

const FIND_DOCTOR_BD_2021 = {
  label: "Find Doctor BD (May 2021)",
  url: "https://finddoctorbd.blogspot.com/2021/05/ambulance-services-list-in-sylhet.html",
};

// Old 0821-xxxxxx landlines from the same lists are left out: Sylhet landlines
// have moved to the 02-99… format, so those are likely dead.
export const AMBULANCES: DirectoryEntry[] = [
  {
    name: "Ambulance Service Sylhet",
    kind: "Private",
    address: "New Medical Road, Kajolshah, Sylhet",
    phones: [{ label: "Day and night, 24 hours", number: "01715778689" }],
    website: "https://ambulanceservicesylhet.com/",
    note: "The provider's own website (last updated 2023–24).",
    confidence: "directory",
    sources: [{ label: "ambulanceservicesylhet.com", url: "https://ambulanceservicesylhet.com/" }],
  },
  {
    name: "Al-Madina Ambulance",
    kind: "Private",
    address: "Sylhet",
    phones: [{ label: "Phone", number: "01711399742" }],
    confidence: "directory",
    sources: [FIND_DOCTOR_BD_2021],
  },
  {
    name: "Metropolitan Ambulance Service",
    kind: "Private",
    address: "Sylhet",
    phones: [{ label: "Phone", number: "01711069486" }],
    confidence: "directory",
    sources: [FIND_DOCTOR_BD_2021],
  },
  {
    name: "Sylhet Patient Care Ambulance Service",
    kind: "Private",
    address: "Medical College Road, Sylhet-3100",
    phones: [{ label: "Phone", number: "01721520585" }],
    confidence: "directory",
    sources: [FIND_DOCTOR_BD_2021],
  },
];

// "+8809610848484" / "01779900016" -> a tel: link.
export function telHref(number: string): string {
  const digits = number.replace(/[^\d+]/g, "");
  if (digits.startsWith("+")) return `tel:${digits}`;
  if (digits.startsWith("0") && digits.length >= 10) return `tel:+88${digits}`;
  return `tel:${digits}`;
}
