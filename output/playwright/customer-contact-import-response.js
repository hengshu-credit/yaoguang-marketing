async (page) => {
  await page.route('http://127.0.0.1:8081/api/contacts.import', async route => {
    const data = route.request().postDataJSON();
    const contact = data.contacts[0];
    if (contact.email !== 'import-review@example.com' || contact.external_id !== '000123456789012345678' || contact.first_name !== '小明' || contact.phone !== '0013800000000') throw new Error('Contact field mapping changed values');
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ operations: [{ email: contact.email, action: 'created' }] }) });
  });
}
