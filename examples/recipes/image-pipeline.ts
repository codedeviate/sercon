// Recipe: load the medium sample image, build a grayscale thumbnail, save PNG+JPEG.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

const im = image.decode(await fs.readBytes(data("medium.png")));
const thumb = im.resize(200, 0).grayscale();
runtime.assert.equal(thumb.width, 200, "thumb width");
const pngPath = `${tmp}/sercon-thumb.png`;
const jpgPath = `${tmp}/sercon-thumb.jpg`;
await fs.writeBytes(pngPath, thumb.bytes("png"));
await fs.writeBytes(jpgPath, thumb.bytes("jpeg"));
runtime.log("image-pipeline: wrote", pngPath, "and", jpgPath, `(${thumb.width}x${thumb.height} grayscale)`);
