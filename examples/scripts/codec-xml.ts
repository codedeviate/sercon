// codec.xml — serialize a value to XML and parse it back.
// @-prefixed keys are attributes, #text is element text, arrays are repeated
// siblings. The top object's single key names the root element.

const value = {
  catalog: {
    "@version": "1",
    book: [
      { "@id": "b1", title: "Go in Practice" },
      { "@id": "b2", title: "The TypeScript Handbook" },
    ],
  },
};

const xml = codec.xml.encode(value, { indent: "  ", declaration: true });
runtime.log(xml);

const back = codec.xml.decode(xml);
if (JSON.stringify(back) !== JSON.stringify(value)) {
  throw new Error("codec-xml round-trip failed: " + JSON.stringify(back));
}
runtime.log("codec-xml self-test PASS");
