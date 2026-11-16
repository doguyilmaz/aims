export default {
  fetch(request) {
    const url = new URL(request.url);
    if (url.hostname !== "aims.apps.doguyilmaz.com") {
      return new Response("Not found", { status: 404 });
    }
    url.protocol = "https:";
    url.host = "aims.doguyilmaz.com";
    return Response.redirect(url.href, 301);
  },
};
