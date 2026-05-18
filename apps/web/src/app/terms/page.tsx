import type { Metadata } from "next";
import Link from "next/link";

import { LegalList, LegalPage, LegalSection } from "@/components/LegalPage";

export const metadata: Metadata = {
  title: "Terms and Conditions | SLAI",
  description: "Terms and Conditions for SLAI prepaid AI API credits",
};

const updated = "May 18, 2026";

export default function TermsPage() {
  return (
    <LegalPage
      eyebrow="Terms"
      title="Terms and Conditions"
      updated={updated}
      description="These Terms govern access to SLAI, a prepaid AI API credits platform for developers. They are written for a Cambodia-governed service that may be used globally."
    >
      <LegalSection title="1. Operator And Contacts">
        <p>
          SLAI is operated by SLAI, the prepaid AI API credits platform
          available through slai.shop and related SLAI services. For legal
          notices, contact <strong>legal@slai.shop</strong>. For privacy
          requests, contact <strong>privacy@slai.shop</strong>.
        </p>
        <p>
          These Terms are an operational draft and do not replace advice from
          qualified counsel. If a signed agreement applies to your use of SLAI,
          that agreement controls where it conflicts with these Terms.
        </p>
      </LegalSection>

      <LegalSection title="2. Eligibility And Accounts">
        <p>
          SLAI is intended for developers and businesses that are at least 18
          years old and able to form a binding agreement. You are responsible
          for account activity, user credentials, and keeping your account
          information accurate.
        </p>
        <LegalList
          items={[
            "You may not create accounts using false, misleading, or unauthorized information.",
            "You must protect passwords, session access, and any server credentials used with SLAI.",
            "SLAI may suspend or close accounts that breach these Terms, create security risk, or create payment or abuse risk.",
          ]}
        />
      </LegalSection>

      <LegalSection title="3. Developer API Use">
        <p>
          SLAI creates and manages API keys for use with supported AI service
          integrations. API keys are intended for server-side use from trusted
          backend systems, not for public client-side code, mobile apps, browser
          bundles, or public repositories.
        </p>
        <LegalList
          items={[
            "You are responsible for requests sent with your API key, including usage charges and misuse caused by leaked keys.",
            "SLAI may rotate, revoke, suspend, or resume API keys to protect the service or enforce account status.",
            "SLAI may limit the number of active API keys available to an account unless additional key management is enabled.",
          ]}
        />
      </LegalSection>

      <LegalSection title="4. Prohibited Use">
        <p>You must not use SLAI or connected AI providers to:</p>
        <LegalList
          items={[
            "violate laws, regulations, third-party rights, or payment network rules;",
            "send malware, spam, abusive traffic, or attempts to bypass technical limits;",
            "reverse engineer, scrape, overload, or disrupt SLAI, payment systems, connected AI services, or provider services;",
            "submit sensitive, regulated, or high-risk data unless SLAI has separately approved that use in writing;",
            "resell, sublicense, or share account or API access outside your organization without SLAI approval.",
          ]}
        />
      </LegalSection>

      <LegalSection title="5. Third-Party AI Services">
        <p>
          SLAI is the prepaid credits, billing, key ownership, and account
          layer. Third-party AI service providers and related infrastructure
          remain separate services. AI requests may be transmitted to and
          processed by those third parties. Their availability, model behavior,
          pricing signals, latency, and safety systems can affect SLAI usage.
        </p>
        <p>
          SLAI does not guarantee that model outputs are accurate, complete,
          safe, or fit for a specific purpose. You are responsible for reviewing
          outputs and for how you use them.
        </p>
      </LegalSection>

      <LegalSection title="6. Prepaid Credits And Billing">
        <p>
          SLAI credits represent prepaid usage value inside SLAI. Credits are
          not bank deposits, stored value outside SLAI, currency, or
          transferable property. Credits never expire unless required
          differently by law or a signed agreement.
        </p>
        <LegalList
          items={[
            "Credits and money are tracked using integer units in the SLAI ledger.",
            "Every balance mutation is recorded through ledger entries such as payment credits, usage debits, admin adjustments, refunds, and bonuses.",
            "Usage billing is asynchronous. SLAI syncs provider usage records and debits your balance after usage occurs.",
            "If synced usage makes your balance zero or negative, SLAI may suspend your API key until credits are added or the issue is resolved.",
          ]}
        />
      </LegalSection>

      <LegalSection title="7. Bakong KHQR Checkout And Payment Confirmation">
        <p>
          SLAI package purchases may use Bakong KHQR checkout through a payment
          provider service. A checkout is credited only after SLAI verifies
          payment confirmation, including amount, currency, payment reference,
          and provider status. Displaying a checkout screen or QR code does not
          add credits by itself.
        </p>
        <p>
          Some legacy or fallback checkouts may require proof upload and admin
          review. SLAI may reject unclear, mismatched, duplicate, expired, or
          unverifiable payments.
        </p>
      </LegalSection>

      <LegalSection title="8. Refunds And Finality">
        <p>
          Payments are final once credits are issued or used, except where a
          refund is required by applicable law or SLAI confirms a duplicate,
          mistaken, or failed payment. SLAI may review refund requests case by
          case, and approved refunds may deduct related credits from your
          account.
        </p>
      </LegalSection>

      <LegalSection title="9. Availability, Changes, And Termination">
        <p>
          SLAI may change, pause, limit, or discontinue features, packages,
          payment methods, provider integrations, prices, or account access.
          SLAI may update these Terms by posting a newer version. Continued use
          after an update means you accept the updated Terms.
        </p>
        <p>
          You may stop using SLAI at any time. SLAI may suspend or terminate
          access for breach, security risk, nonpayment, unlawful use, provider
          restrictions, or operational risk.
        </p>
      </LegalSection>

      <LegalSection title="10. Disclaimers And Liability Limits">
        <p>
          SLAI is provided on an as-is and as-available basis to the maximum
          extent permitted by law. SLAI disclaims implied warranties, including
          merchantability, fitness for a particular purpose, non-infringement,
          and uninterrupted availability.
        </p>
        <p>
          To the maximum extent permitted by law, SLAI will not be liable for
          indirect, incidental, special, consequential, exemplary, or punitive
          damages, lost profits, lost data, lost business, provider outages,
          model outputs, or unauthorized API key use. SLAI total liability for
          claims relating to the service will not exceed the amount you paid to
          SLAI for unused credits in the three months before the event giving
          rise to the claim, unless applicable law requires otherwise.
        </p>
      </LegalSection>

      <LegalSection title="11. Governing Law And Disputes">
        <p>
          These Terms are governed by the laws of Cambodia, without regard to
          conflict-of-law rules. Disputes will be handled by the competent
          courts of Cambodia unless SLAI and you agree to arbitration or another
          forum in writing.
        </p>
      </LegalSection>

      <LegalSection title="12. Related Policies">
        <p>
          SLAI handles personal data as described in the{" "}
          <Link
            className="font-semibold text-blue-700 hover:text-blue-800"
            href="/privacy"
          >
            Privacy Policy
          </Link>
          .
        </p>
      </LegalSection>
    </LegalPage>
  );
}
