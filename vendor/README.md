# Local preview dependencies

All browser scripts are loaded on demand from this directory. No CDN is used.

| Package | Version | Upstream | License selected |
| --- | --- | --- | --- |
| mammoth | 1.13.0 | https://github.com/mwilliamson/mammoth.js | BSD-2-Clause |
| marked | 18.1.1 | https://github.com/markedjs/marked | MIT |
| DOMPurify | 3.4.17 | https://github.com/cure53/DOMPurify | Apache-2.0 (dual license) |
| JSZip | 3.10.2 | https://github.com/Stuk/jszip | MIT (dual license) |
| cfb | 1.2.2 | https://github.com/SheetJS/js-cfb | Apache-2.0 |

Unmodified upstream distributions; license texts are retained in each package directory.
Mammoth's standalone browser bundle contains its bundled dependencies and license notices.
To reproduce: install these exact versions with npm outside the site, copy the browser
artifacts and package LICENSE files here. JSZip's expanded size metadata is checked
before Office XML extraction (60MB / 3000 entries); encrypted or malformed files may
fail and always retain the download option. Legacy Office readers extract text only.
