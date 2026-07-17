// text.markdown.toHtml — render Markdown to an HTML string (pure-Go goldmark).
const html = text.markdown.toHtml("# Hi\n\n- a\n- b");
runtime.assert.ok(html.includes("<h1>Hi</h1>"), "heading");
runtime.assert.ok(html.includes("<li>a</li>"), "list item");

// GFM is on by default — a pipe table renders to <table>.
const table = text.markdown.toHtml("| a | b |\n| - | - |\n| 1 | 2 |\n");
runtime.assert.ok(table.includes("<table>"), "gfm table");

// Opt out of GFM, or turn newlines into <br>.
const plain = text.markdown.toHtml("| a | b |\n| - | - |\n", { gfm: false });
runtime.assert.ok(!plain.includes("<table>"), "gfm disabled");
const br = text.markdown.toHtml("a\nb", { hardBreaks: true });
runtime.assert.ok(br.includes("<br>"), "hard breaks");

runtime.log("text.markdown.toHtml OK");
