import re
import timeit

text = "Your sample text with emails, urls, and phone numbers..."

patterns = [
    r'\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b',
    r'https?://(?:[-\w.]|(?:%[\da-fA-F]{2}))+',
    r'\b\d{3}[-.]\d{3}[-.]\d{4}\b'
]

def find_with_individual_patterns(text):
    matches = []
    for pattern in patterns:
        matches.extend(re.findall(pattern, text))
    return matches

combined_pattern = '|'.join(patterns)
def find_with_combined_pattern(text):
    return re.findall(combined_pattern, text)

# Time individual patterns
time_individual = timeit.timeit(lambda: find_with_individual_patterns(text), number=1000)
print(f"Time with individual patterns: {time_individual:.6f} seconds")

# Time combined pattern
time_combined = timeit.timeit(lambda: find_with_combined_pattern(text), number=1000)
print(f"Time with combined pattern: {time_combined:.6f} seconds")
