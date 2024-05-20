import random
import string

# List of secret key prefixes
secret_key_prefixes = ['secret_key', 'access_token', 'api_key']

# Function to generate a random secret key
def generate_secret_key(prefix):
    key_length = random.randint(16, 32)
    key_chars = string.ascii_letters + string.digits
    key = ''.join(random.choice(key_chars) for _ in range(key_length))
    return f"{prefix}={key}"

# Function to generate a log entry with a secret key
def generate_log_entry():
    prefix = random.choice(secret_key_prefixes)
    secret_key = generate_secret_key(prefix)
    log_entry = f"[{random.randint(1000, 9999)}] This is a log entry with a {secret_key}"
    return log_entry

# Generate synthetic log data
num_log_entries = 1000  # Adjust the number of log entries as needed
synthetic_log_data = "\n".join(generate_log_entry() for _ in range(num_log_entries))

# Save the synthetic log data to a file
with open("synthetic_log_data.txt", "w", encoding="utf-8") as file:
    file.write(synthetic_log_data)