export const dynamic = "force-dynamic";

export function GET() {
  return Response.json(
    {
      googleClientId: process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID ?? process.env.GOOGLE_CLIENT_ID ?? ""
    },
    {
      headers: {
        "Cache-Control": "no-store"
      }
    }
  );
}
