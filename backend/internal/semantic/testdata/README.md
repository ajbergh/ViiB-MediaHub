# Local embedding reference fixtures

`local_embedding_reference.json` was generated independently with NumPy 2.5.4,
Hugging Face tokenizers 0.23.3 and safetensors 0.8.0 using the pinned
`minishlab/potion-base-8M` revision recorded in `embedding_local.go`.

For each input, load the upstream `tokenizer.json`, call
`tokenizer.encode(text, add_special_tokens=False)`, remove ID 1 (`[UNK]`),
mean-pool `embeddings[ids]` from `model.safetensors`, and L2-normalize.
No truncation is applied. These are reference outputs, not outputs of ViiB's
Go implementation. Cases cover accents, punctuation, Chinese characters,
special tokens, control characters, and music descriptions.

The opt-in real-model Go test compares every token ID and all 256 vector
components (absolute tolerance 1e-5), then checks two music relevance orderings.
Ordinary unit tests verify small fixtures and download/cache failure behavior
without acquiring model weights. Set `VIIB_LOCAL_MODEL_TEST_DIR` to enable
real-model tests. No Python dependencies are required to run ViiB or its tests.
