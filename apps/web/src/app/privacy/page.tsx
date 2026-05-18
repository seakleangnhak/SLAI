import type { Metadata } from "next";
import Link from "next/link";

import { LegalList, LegalPage, LegalSection } from "@/components/LegalPage";

export const metadata: Metadata = {
  title: "Privacy Policy | SLAI",
  description: "Privacy Policy for SLAI prepaid AI API credits",
};

const updated = "May 18, 2026";

export default function PrivacyPage() {
  return (
    <LegalPage
      eyebrow="Privacy"
      title="Privacy Policy"
      updated={updated}
      description="This Privacy Policy explains how SLAI collects, uses, shares, and retains information for its prepaid AI API credits platform."
    >
      <LegalSection title="1. Controller And Contacts">
        <p>
          SLAI is operated by SLAI, the prepaid AI API credits platform
          available through slai.shop and related SLAI services. For privacy
          requests, contact <strong>privacy@slai.shop</strong>. For legal
          notices, contact <strong>legal@slai.shop</strong>.
        </p>
        <p>
          This is an operational draft intended to reflect the current product.
          It should be reviewed by qualified counsel before production launch.
        </p>
      </LegalSection>

      <LegalSection title="2. Information We Collect">
        <LegalList
          items={[
            "Account data, including email address, role, account status, balance policy, timestamps, and profile/session metadata.",
            "Authentication data, including password hashes and HttpOnly session cookie records. SLAI does not store plaintext passwords.",
            "API key metadata, including key names, status, display prefix, third-party service reference ids, hash, timestamps, and revocation/suspension data. SLAI does not store raw API keys after showing them once.",
            "Payment and checkout data, including packages, amounts, currency, credit units, Bakong KHQR references, payment processor ids/statuses, payment callbacks, transaction ids, expiry data, and payment review details.",
            "Optional proof uploads for fallback or manual review, including receipt files, file names, MIME types, file size, file hashes, user transaction references, and notes.",
            "Credit ledger and balance data, including payment credits, usage debits, admin adjustments, refund debits, bonus credits, balance after each ledger mutation, reasons, and metadata.",
            "Usage data, including synced third-party usage records, model, provider, token counts, cost units, occurrence time, external reference ids, billing status, and raw billing or usage payloads where needed for billing and audit.",
            "Admin and security records, including admin audit logs, user status changes, API key actions, payment reviews, operational logs, IP-derived request context if captured by infrastructure, and error diagnostics.",
          ]}
        />
      </LegalSection>

      <LegalSection title="3. AI Request And Provider Processing">
        <p>
          SLAI is a business and billing layer for prepaid AI usage. Your model
          requests may be sent to third-party AI service providers and related
          infrastructure for processing. SLAI may store billing and usage
          metadata and raw billing or usage records from those providers, and
          those providers may separately process request content under their own
          terms and policies.
        </p>
        <p>
          Do not submit sensitive, regulated, confidential, health, payment
          card, government identification, or high-risk personal data through
          SLAI-connected AI requests unless SLAI has approved that use in a
          separate written agreement.
        </p>
      </LegalSection>

      <LegalSection title="4. How We Use Information">
        <LegalList
          items={[
            "Create and secure accounts, sessions, and API keys.",
            "Provide prepaid package checkout, Bakong KHQR payment confirmation, proof review, and crediting.",
            "Maintain ledger-backed balances and bill usage asynchronously from provider usage records.",
            "Suspend, resume, rotate, or revoke API keys based on account status, balance, security risk, or admin action.",
            "Detect duplicate events, payment conflicts, misuse, fraud, service abuse, and security incidents.",
            "Provide dashboards, support, troubleshooting, audit logs, accounting records, and legal compliance.",
            "Improve reliability, pricing, billing accuracy, and product operations.",
          ]}
        />
      </LegalSection>

      <LegalSection title="5. Sharing And Third Parties">
        <p>SLAI may share information with:</p>
        <LegalList
          items={[
            "Third-party AI service providers and related infrastructure, to process AI requests, manage API access, and synchronize usage.",
            "Payment processors, Bakong KHQR, banks, or payment-related services, to create checkouts, confirm payments, prevent duplicates, and resolve payment issues.",
            "Hosting, database, storage, logging, analytics, email, and security vendors that help operate SLAI.",
            "Administrators and support staff who need access to operate billing, support, payment review, security, and audit workflows.",
            "Authorities, courts, counterparties, or advisors where required by law, to protect rights and safety, or to resolve disputes.",
          ]}
        />
        <p>
          SLAI does not sell personal information as a standalone business
          model.
        </p>
      </LegalSection>

      <LegalSection title="6. Cookies And Local Storage">
        <p>
          SLAI uses essential HttpOnly session cookies for authentication. The
          web app may use localStorage to remember theme preference. We do not
          currently plan a separate cookie consent banner for this version
          because these uses are essential or preference-based and not
          cross-site advertising cookies.
        </p>
      </LegalSection>

      <LegalSection title="7. Retention">
        <p>
          SLAI retains account, payment, ledger, usage, proof, security, and
          audit records for as long as needed to provide the service, maintain
          accurate balances, meet accounting and tax obligations, prevent fraud
          and abuse, resolve disputes, enforce agreements, support security, and
          comply with law. Deletion requests are honored where feasible, but
          some records may need to be retained or de-identified for legal,
          accounting, security, audit, or ledger integrity reasons.
        </p>
      </LegalSection>

      <LegalSection title="8. Security">
        <p>
          SLAI uses safeguards such as password hashing, HttpOnly session
          cookies, API key hashing, limited raw API key display, signed payment
          callbacks, ledger idempotency, access controls, and audit logs. No
          system is perfectly secure, and you are responsible for protecting
          credentials and API keys.
        </p>
      </LegalSection>

      <LegalSection title="9. International Processing">
        <p>
          SLAI may process and store information in Cambodia and other countries
          where SLAI, hosting providers, payment services, AI service providers,
          or infrastructure vendors operate. Those countries may have different
          data protection rules than your location.
        </p>
      </LegalSection>

      <LegalSection title="10. Your Requests And Choices">
        <p>
          Depending on your location, you may have rights to request access,
          correction, deletion, portability, restriction, objection, or
          information about how your data is used. Send requests to{" "}
          <strong>privacy@slai.shop</strong>. SLAI may need to verify your
          identity and may retain certain records where required or permitted by
          law.
        </p>
      </LegalSection>

      <LegalSection title="11. Minors">
        <p>
          SLAI is intended for developers and businesses that are at least 18
          years old. SLAI is not intended for children, and we do not knowingly
          collect information from children.
        </p>
      </LegalSection>

      <LegalSection title="12. Changes">
        <p>
          SLAI may update this Privacy Policy as the product, providers, laws,
          or operations change. The updated version will be posted on this page
          with a new last-updated date.
        </p>
      </LegalSection>

      <LegalSection title="13. Related Terms">
        <p>
          Your use of SLAI is also governed by the{" "}
          <Link
            className="font-semibold text-blue-700 hover:text-blue-800"
            href="/terms"
          >
            Terms and Conditions
          </Link>
          .
        </p>
      </LegalSection>
    </LegalPage>
  );
}
