// Recipe: hide a password-protected secret inside the small sample image,
// write the stego PNG to tmp, then recover it to prove the round-trip.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

const carrier = await fs.readBytes(data("small.png"));
const secret = "rendezvous at 0900";
const stego = image.stego.embed(carrier, secret, { password: "p@ss" });
const stegoPath = `${tmp}/sercon-stego.png`;
await fs.writeBytes(stegoPath, stego.bytes);
const recovered = image.stego.extract(await fs.readBytes(stegoPath), { password: "p@ss" });
runtime.assert.equal(recovered, secret, "stego round-trip");
runtime.log("stego-hide: wrote", stegoPath, "— recovered secret:", recovered);
