export default function (req, res) {
    const { url, method, body, headers } = req;
    const targetUrl = `http://13.218.75.79:8090${url.replace('/api/proxy', '')}`;
    
    // Forward the request to your EC2 instance
    fetch(targetUrl, {
      method,
      headers: {
        'Content-Type': 'application/json',
      },
      body: method !== 'GET' && method !== 'HEAD' ? JSON.stringify(body) : undefined,
    })
    .then(response => response.json())
    .then(data => {
      res.status(200).json(data);
    })
    .catch(error => {
      res.status(500).json({ error: error.message });
    });
  }