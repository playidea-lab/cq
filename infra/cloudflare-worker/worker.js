// Cloudflare Worker: proxy /.well-known/* requests on mcp.pilab.kr
// to the Supabase Auth server, enabling ChatGPT OAuth discovery.

const SUPABASE_AUTH = "https://fhuomvsswxiwbfqjsgit.supabase.co";

export default {
  async fetch(request) {
    const url = new URL(request.url);

    // OAuth Authorization Server Metadata
    // ChatGPT requests: /.well-known/oauth-authorization-server
    // or /.well-known/oauth-authorization-server/{path}
    if (url.pathname.startsWith("/.well-known/oauth-authorization-server")) {
      const targetUrl = `${SUPABASE_AUTH}/.well-known/oauth-authorization-server/auth/v1`;
      const res = await fetch(targetUrl);
      const body = await res.text();
      return new Response(body, {
        status: res.status,
        headers: {
          "Content-Type": "application/json",
          "Access-Control-Allow-Origin": "*",
          "Cache-Control": "no-store",
        },
      });
    }

    // OpenID Configuration
    if (url.pathname.startsWith("/.well-known/openid-configuration")) {
      const targetUrl = `${SUPABASE_AUTH}/auth/v1/.well-known/openid-configuration`;
      const res = await fetch(targetUrl);
      const body = await res.text();
      return new Response(body, {
        status: res.status,
        headers: {
          "Content-Type": "application/json",
          "Access-Control-Allow-Origin": "*",
          "Cache-Control": "no-store",
        },
      });
    }

    // OAuth Protected Resource Metadata (fallback — also handled by Edge Function)
    if (url.pathname.startsWith("/.well-known/oauth-protected-resource")) {
      return new Response(
        JSON.stringify({
          resource: "https://mcp.pilab.kr/functions/v1/mcp-server",
          authorization_servers: [
            "https://fhuomvsswxiwbfqjsgit.supabase.co/auth/v1",
          ],
          bearer_methods_supported: ["header"],
          scopes_supported: ["openid"],
        }),
        {
          headers: {
            "Content-Type": "application/json",
            "Access-Control-Allow-Origin": "*",
            "Cache-Control": "no-store",
          },
        }
      );
    }

    // JWKS
    if (url.pathname.startsWith("/.well-known/jwks.json")) {
      const targetUrl = `${SUPABASE_AUTH}/auth/v1/.well-known/jwks.json`;
      const res = await fetch(targetUrl);
      const body = await res.text();
      return new Response(body, {
        status: res.status,
        headers: {
          "Content-Type": "application/json",
          "Access-Control-Allow-Origin": "*",
        },
      });
    }

    // Anything else under /.well-known — pass through
    return new Response("Not found", { status: 404 });
  },
};
