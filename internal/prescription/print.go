package prescription

import (
	"bytes"
	"encoding/base64"
	"html/template"
	"io"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// Render writes a printable HTML page for p. Browsers render Bangla text correctly
// and "Print → Save as PDF" produces a PDF. verifyURL is encoded in a QR code so a
// pharmacy can scan it to check the prescription.
func Render(w io.Writer, p Prescription, verifyURL string) error {
	png, err := qrcode.Encode(verifyURL, qrcode.Medium, 160)
	if err != nil {
		return err
	}
	return printTemplate.Execute(w, map[string]any{
		"P":         p,
		"QR":        template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png)),
		"VerifyURL": verifyURL,
		"Issued":    p.IssuedAt.In(Dhaka).Format("02 Jan 2006, 3:04 PM"),
		"Revoked":   p.Status == StatusRevoked,
	})
}

// RenderString is Render into a string, for tests.
func RenderString(p Prescription, verifyURL string) (string, error) {
	var buf bytes.Buffer
	err := Render(&buf, p, verifyURL)
	return buf.String(), err
}

var printTemplate = template.Must(template.New("rx").Funcs(template.FuncMap{
	"deref": func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	},
	"date": func(t time.Time) string { return t.In(Dhaka).Format("02 Jan 2006") },
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Prescription {{.P.Code}}</title>
<style>
  :root { --ink: #1a1a1a; --muted: #5b6470; --line: #d5d9de; --accent: #0b6e4f; }
  * { box-sizing: border-box; }
  body { margin: 0; background: #f3f4f6; color: var(--ink);
         font: 15px/1.5 "Noto Sans Bengali", "Hind Siliguri", "Segoe UI", system-ui, sans-serif; }
  .page { max-width: 820px; margin: 24px auto; background: #fff; padding: 32px 36px;
          border: 1px solid var(--line); position: relative; }
  header { display: flex; justify-content: space-between; gap: 16px;
           border-bottom: 3px solid var(--accent); padding-bottom: 12px; }
  h1 { margin: 0; font-size: 22px; }
  .muted { color: var(--muted); font-size: 13px; }
  .qr { text-align: center; font-size: 11px; color: var(--muted); }
  .qr img { width: 110px; height: 110px; display: block; }
  .patient { display: flex; flex-wrap: wrap; gap: 8px 24px; padding: 10px 0;
             border-bottom: 1px solid var(--line); font-size: 14px; }
  .body { display: grid; grid-template-columns: 34% 1fr; gap: 24px; padding-top: 16px; }
  .left { border-right: 1px solid var(--line); padding-right: 16px; }
  h2 { font-size: 13px; text-transform: uppercase; letter-spacing: .04em; color: var(--accent); margin: 14px 0 4px; }
  .rx { font: italic 700 28px Georgia, serif; color: var(--accent); }
  ol { padding-left: 20px; margin: 4px 0; }
  ol li { margin-bottom: 10px; }
  .med { font-weight: 600; }
  .dose { color: var(--muted); font-size: 14px; }
  p { margin: 4px 0; white-space: pre-wrap; }
  footer { margin-top: 32px; display: flex; justify-content: space-between; align-items: flex-end;
           border-top: 1px solid var(--line); padding-top: 12px; font-size: 12px; color: var(--muted); }
  .sign { text-align: right; color: var(--ink); font-size: 14px; }
  .revoked { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center;
             pointer-events: none; }
  .revoked span { transform: rotate(-24deg); font: 700 72px system-ui; color: rgba(200, 30, 30, .25);
                  border: 8px solid rgba(200, 30, 30, .25); padding: 8px 24px; }
  @media (max-width: 640px) {
    .page { margin: 0; padding: 20px 16px; border: 0; }
    .body { grid-template-columns: 1fr; }
    .left { border-right: 0; padding-right: 0; }
  }
  @media print {
    body { background: #fff; }
    .page { margin: 0; border: 0; max-width: none; }
  }
</style>
</head>
<body>
<div class="page">
  {{if .Revoked}}<div class="revoked"><span>REVOKED</span></div>{{end}}
  <header>
    <div>
      <h1>{{.P.DoctorName}}</h1>
      {{with deref .P.DoctorDegrees}}<div>{{.}}</div>{{end}}
      {{with deref .P.DoctorSpecialization}}<div class="muted">{{.}}</div>{{end}}
      <div class="muted">BMDC Reg. No: {{.P.DoctorBMDC}}</div>
    </div>
    <div class="qr">
      <img src="{{.QR}}" alt="QR code to verify this prescription">
      <div>{{.P.Code}}</div>
    </div>
  </header>

  <div class="patient">
    <div><strong>Patient:</strong> {{.P.PatientName}}</div>
    {{with deref .P.PatientAge}}<div><strong>Age:</strong> {{.}}</div>{{end}}
    {{with deref .P.PatientGender}}<div><strong>Gender:</strong> {{.}}</div>{{end}}
    <div><strong>Date:</strong> {{.Issued}}</div>
  </div>

  <div class="body">
    <div class="left">
      {{with deref .P.ChiefComplaints}}<h2>Chief complaints</h2><p>{{.}}</p>{{end}}
      {{with deref .P.Diagnosis}}<h2>Diagnosis</h2><p>{{.}}</p>{{end}}
      {{if .P.Investigations}}<h2>Investigations</h2>
        <ol>{{range .P.Investigations}}<li>{{.}}</li>{{end}}</ol>{{end}}
    </div>
    <div>
      <div class="rx">&#8478;</div>
      {{if .P.Medicines}}<ol>
        {{range .P.Medicines}}<li>
          <div class="med">{{with .Form}}{{.}} {{end}}{{.Name}}{{with .Strength}} {{.}}{{end}}</div>
          <div class="dose">{{.Dosage}} &middot; {{.Duration}}{{with .Instructions}} &middot; {{.}}{{end}}</div>
        </li>{{end}}
      </ol>{{end}}
      {{with deref .P.Advice}}<h2>Advice</h2><p>{{.}}</p>{{end}}
      {{with .P.FollowUpDate}}<h2>Follow-up</h2><p>{{.}}</p>{{end}}
    </div>
  </div>

  <footer>
    <div>Digital prescription &middot; Code {{.P.Code}}<br>Verify: {{.VerifyURL}}</div>
    <div class="sign">{{.P.DoctorName}}<br><span class="muted">Digitally issued</span></div>
  </footer>
</div>
</body>
</html>
`))
