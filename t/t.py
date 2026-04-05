import requests
base_url = "https://openrouter.ai/api/v1"
url = f"{base_url}/agent/4e5739f8-8e35-4291-bb86-1568965a627c/stream"  
with requests.get(url, stream=True) as r:
    r.raise_for_status()
    print("Connected. Listening for events...\n")
    for line in r.iter_lines(decode_unicode=True):
        if line:
            print(line)