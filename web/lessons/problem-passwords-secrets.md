## Why this is a database problem

A [database](#/lesson/what-is-a-database) that is available and consistent still fails its job if it stores secrets badly. Password dumps, key leaks and "please delete my account" requests are interview staples and production incidents. Treat the data, the keys and the copies as three different problems.

> **Key idea:** Passwords are not confidential data you encrypt. They are low-entropy secrets you **hash** with a slow, salted, one-way function. Encryption is for data you must read back. Hashing is for data you must only **verify**.

## How you store passwords

Never store a password in plaintext. Never store it under reversible encryption (AES of the password, "so we can email it back"). If the key leaks, every account leaks. If an operator can decrypt, so can an attacker who becomes that operator.

Hash with a **password-based key-derivation function** designed to be slow and, preferably, memory-hard:

| Function | Use it? | Notes |
|---|---|---|
| **Argon2id** | Preferred | Winner of the Password Hashing Competition. Mixes data-dependent and data-independent memory access. Parameters: memory, iterations, parallelism. |
| **bcrypt** | Acceptable | Widely deployed, well studied. 72-byte password limit — pre-hash only if you understand the trade-off. Cost (work factor) is a power of two. |
| scrypt / PBKDF2 | Legacy | scrypt is memory-hard; PBKDF2 is iterated HMAC and weaker against GPUs. Migrate off when you can. |
| MD5, SHA-1, SHA-256 alone | Never | Fast cryptographic hashes. The wrong tool. |

**Salt.** A unique random value per password, stored next to the hash. It stops rainbow tables and stops two users with the same password having the same hash.

**Pepper.** A secret that is **not** in the database — typically in a KMS or secret manager — mixed into the hash. A stolen table is then useless without the pepper. Rotate by re-hashing on next login.

**Work factor.** Tune so one hash takes roughly 100–500 ms on your production hardware (a common starting point is ~200 ms). Follow the current OWASP Password Storage Cheat Sheet for Argon2id memory/iteration figures, then measure. As GPUs get cheaper you raise the cost; store the parameters with the hash so you can upgrade.

![Password goes through a unique salt, an optional pepper and a slow KDF; a fast hash alone falls to GPUs.](diagrams/problem-passwords-secrets-hashing-pipeline.svg)

### Why MD5, SHA-1 and SHA-256-alone fail

SHA-256 is an excellent integrity hash. It is a terrible password hash because it is **fast**. A single modern GPU tries **billions** of SHA-256 (or MD5) guesses per second. Unsalted hashes fall to precomputed rainbow tables. "SHA-256 plus a salt" is still too fast: the salt kills rainbow tables, not the GPU.

A password KDF exists to make each guess expensive in time and RAM, so offline cracking of a dumped table becomes uneconomic for ordinary passwords.

### Timing-safe compare

When a user logs in you hash the attempt and compare it to the stored hash. A naive `==` or `bytes.Equal` can return early on the first differing byte and leak the hash through timing. Use a constant-time compare (`subtle.ConstantTimeCompare` in Go, `hmac.Equal` in the Python stdlib, `crypto.timingSafeEqual` in Node).

```go
import (
    "crypto/rand"
    "crypto/subtle"
    "golang.org/x/crypto/argon2"
)

func hashPassword(password string, pepper []byte) (salt, hash []byte, err error) {
    salt = make([]byte, 16)
    if _, err = rand.Read(salt); err != nil {
        return nil, nil, err
    }
    // Tune memory/iterations to ~200 ms on your hardware; store them with the hash.
    hash = argon2.IDKey([]byte(password), append(salt, pepper...), 2, 19*1024, 1, 32)
    return salt, hash, nil
}

func checkPassword(password string, salt, pepper, stored []byte) bool {
    got := argon2.IDKey([]byte(password), append(salt, pepper...), 2, 19*1024, 1, 32)
    return subtle.ConstantTimeCompare(got, stored) == 1
}
```

### Reset tokens

A reset link is a capability. Generate at least 32 bytes from a CSPRNG, send the raw token **once** over TLS, and store only a hash of it (a fast hash is fine here — the token is already high-entropy). Give it a short TTL (minutes, not days), make it single-use, and invalidate outstanding tokens when the password changes. Do not put the raw token in server logs.

> **Gotcha:** "Hash the email, store the hash, look users up" is not password hashing. Emails are not secrets you verify; they are identifiers you often need to reverse or search. See tokenisation below.

## Encryption: at rest, in transit, in the application

Three layers, three threat models. Stack them; do not pick one.

| Layer | What it protects | What it does not |
|---|---|---|
| **TLS in transit** | Network sniffers, leftover packet captures | A stolen disk, a stolen backup, a compromised app |
| **Volume / disk encryption** | Stolen disks and unshredded hardware | Anyone who can read the mounted filesystem, including the database process |
| **TDE** (transparent data encryption) | Data files and usually WAL on disk, often with keys in a KMS | A query issued by a compromised role; the server decrypts to answer SQL |
| **Application-level encryption** | A stolen replica, a curious DBA, a leaked backup of specific columns | Ad-hoc `WHERE email = ?` unless you also store a blind index |

Cloud disks (EBS, Persistent Disk, Azure Disk) and TDE in Oracle, SQL Server, MySQL and managed Postgres all matter. They are necessary and not sufficient. The SA question is always: **"stolen by whom?"**

**Envelope encryption** is how you manage keys at scale. Generate a random data-encryption key (DEK) per object or tenant, encrypt the data with the DEK, then **wrap** the DEK with a key-encryption key (KEK) that never leaves a KMS. Store ciphertext and wrapped DEK together. To rotate the KEK you re-wrap DEKs; you do not rewrite the data.

![A KMS-held KEK wraps a per-object DEK; ciphertext and the wrapped DEK are stored together so KEK rotation does not rewrite data.](diagrams/problem-passwords-secrets-envelope.svg)

Per-tenant DEKs are how a [multi-tenant](#/lesson/problem-multi-tenant) pool gets something close to silo isolation, and how you crypto-shred a tenant you must forget.

## Tokenisation vs hashing vs encryption

| | Reversible? | Typical use | Lookup? |
|---|---|---|---|
| **Hashing** (KDF or fast) | No | Passwords (slow KDF); high-entropy tokens (fast hash) | Only by hashing the probe |
| **Encryption** | Yes, with the key | Personal data you must read back | Hard unless you also keep a blind index |
| **Tokenisation** | Yes, via a vault | PANs, national IDs: store a token, keep the mapping in a tighter system | The token is the foreign key |

Tokenisation is what PCI-ish designs do with card numbers: the database sees `tok_…`, the vault sees the PAN. Hashing a PAN with SHA-256 is not tokenisation and is often still treated as cardholder data. Do not invent a compliance story in an interview; say what the mechanism does and defer the audit to specialists.

## PII columns, audit, and the right to be forgotten

PII is any data that identifies a person or makes them identifiable: names, emails, phone numbers, IPs, device IDs, precise location, financial and health attributes. Natural keys (email as primary key) leak PII into every foreign key and log line. Prefer surrogates; see [Data modelling](#/lesson/data-modeling).

**Audit logs** record who read or changed what. They are themselves PII and must be retained, protected and eventually expired. `pgaudit` in Postgres and equivalent session auditing in managed offerings exist for this. Do not log passwords, reset tokens or full PANs.

**Erasure is hard** because the row in the primary is the easy copy. The same personal data lives in:

- replicas and delayed standbys
- [base backups and WAL archives](#/lesson/problem-backup-dr)
- analytics copies, lakehouse tables, search indexes
- CDC streams and Kafka topics
- application logs, dumps, support tickets, caches

A `DELETE FROM users WHERE id = $1` is the start of a process, not the process.

Practical pattern: stop processing immediately (soft-delete / ban flag), anonymise or crypto-shred the live row (destroy that user's DEK), push a tombstone through every pipeline, and wait for backup and log retention to drop the last copy. Document the residual window. That residual window is why backup retention is a privacy decision, not only a [recovery](#/lesson/problem-backup-dr) one.

### GDPR and CCPA, at SA altitude

Stay general. You are not giving legal advice.

Both regimes expect a **lawful basis** (or equivalent notice-and-choice story) for holding personal data, **minimisation** (do not store what you do not need), **purpose limitation**, **security**, and a way for a person to **access, correct and request deletion**. GDPR also talks about portability and accountability; CCPA/CPRA adds rights around sale and sharing. Fines, territorial scope and exceptions are for counsel.

What you *can* own as an SA: data-flow maps (where PII lives), retention aligned with backups, encryption and access control, and an erasure runbook that names every copy.

> **Warning:** Secrets that open the database — connection strings, TDE keys, admin passwords — do not belong in committed config files, container images or world-readable ConfigMaps. Put them in a secret manager (Vault, AWS Secrets Manager, External Secrets into Kubernetes) and rotate them. A leaked `.env` in git is a production incident.

## Interview problems

### Problem: How do you store passwords?

> **Real world:** An interviewer asks, with no further prompt, "How do you store passwords?" They want the mechanism and the threat model, not a brand name.

**Symptoms.** The question is really: what happens when the table leaks.

**Diagnose.** Ask what is already in the dump (hashes? salts? peppers?) and whether login compare is constant-time.

**Root causes.** Plaintext, reversible encryption, or a fast hash. Shared salts. Work factor never raised.

**Fix.** Argon2id (or bcrypt), unique salt, optional pepper in KMS, work factor ~200 ms, timing-safe compare, migrate on login.

**Prevent.** Ban new MD5/SHA password columns in review; alert if a users table column looks like hex of 16 or 32 bytes with no parameters prefix.

> **Model answer — what to say in the interview:** I never store plaintext or encrypted passwords. I hash with Argon2id, falling back to bcrypt if that is what the stack already has, with a unique salt per password and a pepper in the KMS. I tune the work factor so a hash takes a few hundred milliseconds, store the parameters with the hash, and compare in constant time. Reset tokens are high-entropy random values; I store a hash of the token, not the token, with a short TTL. Fast hashes like SHA-256 are for integrity, not for passwords — a GPU will burn through them.

### Problem: A user asks to be deleted

> **Real world:** Support files a ticket: "GDPR delete this user." Product expects the row to vanish today. Legal wants a record of what you did. Backups go back 30 days.

**Symptoms.** The live row is gone; the email still appears in warehouse queries, search, logs and last night's backup.

**Diagnose.** Inventory copies: primary, replicas, PITR window, object storage dumps, CDC topics, analytics, logs. Ask how long each is retained.

**Root causes.** Erasure treated as one `DELETE`. Backups and derived stores have no tombstone path. Email used as a key, so it is embedded everywhere.

**Fix.** Immediately stop processing and anonymise or crypto-shred the live record. Fan a deletion event out through every pipeline. Leave backups to expire, or restore-scrub-re-backup only if counsel requires it — that is expensive and rare. Write down the residual window.

**Prevent.** Surrogate keys, per-user or per-tenant DEKs, a single erasure workflow, retention that matches the promise you make to users. See [Backups and DR](#/lesson/problem-backup-dr) and [multi-tenant keys](#/lesson/problem-multi-tenant).

> **Model answer — what to say in the interview:** Deleting the primary row is not erasure. I would disable the account immediately, anonymise or destroy the encryption key for that user's data, and emit a tombstone so replicas, search, CDC and the warehouse drop or hash the identifiers. Backups and WAL archives will still hold the old bytes until retention expires; I would state that window honestly rather than claim the person is gone from every disk. I would not restore-and-scrub thirty days of backups unless legal required it.

```quiz
Q: Why is SHA-256 alone a bad way to store passwords?
+ It is fast, so a stolen table can be attacked at billions of guesses per second
- Good hashes must be 512 bits
- It is not standardised
- It cannot be salted
? SHA-256 is an integrity hash. Password storage needs a slow, memory-hard KDF so offline guessing is expensive.

Q: What is the difference between a salt and a pepper?
- They are two names for the same random prefix
- A pepper is stored in the users table; a salt is not
- A salt is shared across all users; a pepper is unique per row
+ A salt is unique per password and stored with the hash; a pepper is a secret kept outside the database
? The salt defeats rainbow tables. The pepper means a leaked table is still not enough.

Q: You need to rotate the key that protects millions of encrypted rows. What does envelope encryption let you do?
- Re-encrypt every row under a new DEK
- Delete the KMS and generate new plaintext
+ Re-wrap the DEKs with a new KEK in the KMS, without rewriting the ciphertext
- Switch from TLS 1.2 to TLS 1.3
? Data is encrypted with DEKs. Only the wrapped DEKs must change when the KEK rotates.

Q: A reset-password email contains a token. What should the database store?
- The token in plaintext so support can resend it
- The user's new password, encrypted with AES
- The token encrypted with the user's password
+ A hash of a high-entropy token, with a short TTL and single use
? The token is already random, so a fast hash is enough. Storing the raw value lets anyone with table access take over the account.

Q: Why is "we ran DELETE FROM users" not enough when a person asks to be forgotten?
- DELETE is always rolled back by WAL
- GDPR forbids DELETE
+ Replicas, backups, logs, CDC streams and analytics copies still hold the data
- The row will come back from the cache automatically
? Erasure is a process across every copy. The primary is the easy one.

Q: Where should the database connection string live?
- In a YAML file committed next to the schema
+ In a secret manager or the process environment, never in the repo
- Hard-coded in the application binary
- In a comment on the users table
? A leaked repo or image should not be a leaked database. Rotate the secret independently of deploys.
```
