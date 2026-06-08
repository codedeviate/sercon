// Demonstrates services.webdriver — a complete UI login-form test.
// Self-skips when no chromedriver/geckodriver is on PATH.
//
// What this does:
//   1. Connects headlessly to Chrome.
//   2. Loads a self-contained data: URL with a username+password form and a
//      tiny inline script that sets #result's text on submit (no navigation).
//   3. Finds both inputs, types credentials, clicks the submit button.
//   4. Waits for #result to have non-empty text (polls via executeScript).
//   5. Reads the result text via an element handle AND via executeScript.
//   6. runtime.assert that the greeting matches.
//   7. Takes a screenshot (bytes form) and logs its size.
//   8. Quits in a finally block.

if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver-login-flow.");
} else {
  const USERNAME = "alice";
  const PASSWORD = "secret";
  const EXPECTED  = "welcome " + USERNAME;

  // Build the login-form page as a data: URL so no network is needed.
  const html = `
<!DOCTYPE html>
<html>
<head><title>Login Test</title></head>
<body>
  <form id="loginForm">
    <input id="username" type="text"     placeholder="username" />
    <input id="password" type="password" placeholder="password" />
    <button id="submit" type="submit">Login</button>
  </form>
  <div id="result"></div>
  <script>
    document.getElementById('loginForm').addEventListener('submit', function(e) {
      e.preventDefault();
      var u = document.getElementById('username').value;
      document.getElementById('result').textContent = 'welcome ' + u;
    });
  </script>
</body>
</html>`.trim();

  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    // Navigate to the self-contained login page.
    await d.get("data:text/html," + encodeURIComponent(html));
    runtime.log("title:", await d.title());

    // Fill in the form fields.
    const userInput = await d.find("id", "username");
    await userInput.sendKeys(USERNAME);

    const passInput = await d.find("id", "password");
    await passInput.sendKeys(PASSWORD);

    // Submit — click the button (not .submit() so the event fires).
    const btn = await d.find("id", "submit");
    await btn.click();

    // Poll for the result div to become non-empty (max ~3 s, 50 ms cadence).
    let resultText = "";
    for (let attempt = 0; attempt < 60; attempt++) {
      resultText = await d.executeScript(
        "return document.getElementById('result').textContent || ''", []);
      if (resultText) break;
      await runtime.time.sleep(50);
    }

    // Also read via waitFor + element handle to exercise that API path.
    const resultEl = await d.waitFor("id", "result", { timeout: 3000 });
    const handleText = await resultEl.text();
    runtime.log("result via executeScript:", resultText);
    runtime.log("result via element handle:", handleText);

    // Assert correctness.
    runtime.assert.equal(resultText, EXPECTED, "executeScript greeting");
    runtime.assert.equal(handleText, EXPECTED, "element handle greeting");
    runtime.log("login-flow assertion PASS");

    // Screenshot (bytes form) — log its size as a sanity check.
    const shot = await d.screenshot();
    runtime.log("screenshot bytes:", new Uint8Array(shot.bytes).length, shot.format);
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}
