## Page 1

*[Image OCR]
<think>Based on the visual content of the image, I need to extract the text exactly as it appears, preserving the layout and order.

**Top Section:**
- Header: LANDMARK PAPER / NEURIPS 2017
- Title: ATTENTION IS ALL YOU NEED
- Authors: VASWANI ET AL.

**Figure Area:**
- Two diagrams are present. I will extract the text labels within/near them.
- Left Diagram Title: Scaled Dot-Product Attention
- Inside Left Diagram boxes (top to bottom): MatMul, SoftMax, Mask (opt.), Scale, MatMul. Inputs at bottom: Q, K, V. Arrow points up from MatMul.
- Right Diagram Title: Multi-Head Attention
- Inside Right Diagram boxes (top to bottom): Linear, Concat, Scaled Dot-Product Attention (inside a stack of boxes), Linear, Linear, Linear. Inputs at bottom: V, K, Q.
- Figure Caption: Figure 2: (left) Scaled Dot-Product Attention. (right) Multi-Head Attention consists of several attention layers running in parallel.

**Main Text Body:**
- Paragraph starting with "of the values..."
- Section 3.2.1: Scaled Dot-Product Attention
- Paragraph starting "We call our particular attention..." containing "(Figure 2)".
- Paragraph starting "In practice, we compute..."
- Equation (1): Attention(Q, K, V) = softmax(QK^T / sqrt(d_k)) V
- Paragraph starting "The two most commonly used attention functions..." containing citation [2].
- Paragraph starting "While for small values of d_k..." containing citation [3] and footnote marker [4].
- Footnote 4: To illustrate why the dot products get large, assume that the components of q and k are independent random variables with mean 0 and variance 1. Then their dot product, q · k = sum_{i=1}^{d_k} q_i k_i, has mean 0 and variance d_k.
- Section 3.2.2: Multi-Head Attention
- Paragraph starting "Instead of performing a single attention function..."

**Page Number:**
- 4 at the bottom.

I will combine these elements into a continuous text block representing the page content. I need to be careful with line breaks and formatting to match the visual flow.

Let's refine the extraction order:
1. Header info.
2. Title/Author.
3. Figure 2 diagrams and caption.
4. Text paragraph before section 3.2.1.
5. Section 3.2.1 header.
6. Text of 3.2.1 including equation (1).
7. Footnote 4 text.
8. Section 3.2.2 header.
9. Text of 3.2.2.
10. Page number.

Wait, looking closely at the image, the footer with page number '4' is at the very bottom. The footnote is usually part of the main text block or separate. I will include it near where it appears in the text flow or just after the relevant paragraph. In the provided image layout, there isn't a distinct "footnote area" visually separated below the main text column in a standard way, but rather the footnote text is at the bottom left/center under the section 3.2.1 text? No, looking closely, the footnote text "4 To illustrate..." is actually located under section 3.2.1 but above 3.2.2. Wait, let me re-examine.
Actually, looking at the very bottom of the image, there is a page number '4'.
The footnote text starts with "4 To illustrate..." and is located just above the page number, seemingly part of the text column for section 3.2.1? No, it looks like a distinct footnote block at the bottom of the page. Let me look really closely at the bottom left.
Ah, the text "4 To illustrate why the dot products get large..." is actually located *above* the Section 3.2.2 header but indented? No, that's not right.
Let's look at the layout again.
- Section 3.2.1 ends with "...scale the dot products by 1/sqrt(d_k)."
- Then there is a block of text starting with "4 To illustrate why..."
- Then Section 3.2.2 starts.
Actually, looking at the vertical alignment, the footnote text "4 To illustrate..." is aligned to the left margin, below the main paragraph ending with "...extremely small gradients [4].", and above section 3.2.2. This suggests it's a footnote block.
Let's re-read the prompt: "Extract all text from this image... maintaining the original layout and order."

So I will proceed top-to-bottom, left-to-right.

**Text extraction draft:**
LANDMARK PAPER / NEURIPS 2017
ATTENTION IS
ALL YOU NEED
VASWANI ET AL.

Scaled Dot-Product Attention
Multi-Head Attention
(inside diagrams)
MatMul
SoftMax
Mask (opt.)
Scale
MatMul
Q K V
Linear
Concat
Scaled Dot-Product Attention
Linear Linear Linear
V K Q

Figure 2: (left) Scaled Dot-Product Attention. (right) Multi-Head Attention consists of several attention layers running in parallel.

of the values, where the weight assigned to each value is computed by a compatibility function of the query with the corresponding key.

3.2.1 Scaled Dot-Product Attention
We call our particular attention "Scaled Dot-Product Attention" (Figure 2). The input consists of queries and keys of dimension d_k, and values of dimension d_v. We compute the dot products of the query with all keys, divide each by sqrt(d_k), and apply a softmax function to obtain the weights on the values.
In practice, we compute the attention function on a set of queries simultaneously, packed together into a matrix Q. The keys and values are also packed together into matrices K and V. We compute the matrix of outputs as:
Attention(Q, K, V) = softmax(QK^T / sqrt(d_k)) V (1)
The two most commonly used attention functions are additive attention [2], and dot-product (multiplicative) attention. Dot-product attention is identical to our algorithm, except for the scaling factor of 1/sqrt(d_k). Additive attention computes the compatibility function using a feed-forward network with a single hidden layer. While the two are similar in theoretical complexity, dot-product attention is much faster and more space-efficient in practice, since it can be implemented using highly optimized matrix multiplication code.
While for small values of d_k the two mechanisms perform similarly, additive attention outperforms dot product attention without scaling for larger values of d_k [3]. We suspect that for large values of d_k, the dot products grow large in magnitude, pushing the softmax function into regions where it has extremely small gradients [4]. To counteract this effect, we scale the dot products by 1/sqrt(d_k).
4 To illustrate why the dot products get large, assume that the components of q and k are independent random variables with mean 0 and variance 1. Then their dot product, q · k = sum_{i=1}^{d_k} q_i k_i, has mean 0 and variance d_k.
3.2.2 Multi-Head Attention
Instead of performing a single attention function with d_model-dimensional keys, values and queries, we found it beneficial to linearly project the queries, keys and values h times with different, learned linear projections to d_k, d_k and d_v dimensions, respectively. On each of these projected versions of queries, keys and values we then perform the attention function in parallel, yielding d_v-dimensional

4
</think>

LANDMARK PAPER / NEURIPS 2017
ATTENTION IS ALL YOU NEED
VASWANI ET AL.
Scaled Dot-Product Attention
Multi-Head Attention
MatMul
SoftMax
Mask (opt.)
Scale
MatMul
Q K V
Linear
Concat
Scaled Dot-Product Attention
Linear Linear Linear
V K Q
Figure 2: (left) Scaled Dot-Product Attention. (right) Multi-Head Attention consists of several attention layers running in parallel.
of the values, where the weight assigned to each value is computed by a compatibility function of the query with the corresponding key.
3.2.1 Scaled Dot-Product Attention
We call our particular attention "Scaled Dot-Product Attention" (Figure 2). The input consists of queries and keys of dimension d_k, and values of dimension d_v. We compute the dot products of the query with all keys, divide each by sqrt(d_k), and apply a softmax function to obtain the weights on the values.
In practice, we compute the attention function on a set of queries simultaneously, packed together into a matrix Q. The keys and values are also packed together into matrices K and V. We compute the matrix of outputs as:
Attention(Q, K, V) = softmax(QK^T / sqrt(d_k)) V (1)
The two most commonly used attention functions are additive attention [2], and dot-product (multiplicative) attention. Dot-product attention is identical to our algorithm, except for the scaling factor of 1/sqrt(d_k). Additive attention computes the compatibility function using a feed-forward network with a single hidden layer. While the two are similar in theoretical complexity, dot-product attention is much faster and more space-efficient in practice, since it can be implemented using highly optimized matrix multiplication code.
While for small values of d_k the two mechanisms perform similarly, additive attention outperforms dot product attention without scaling for larger values of d_k [3]. We suspect that for large values of d_k, the dot products grow large in magnitude, pushing the softmax function into regions where it has extremely small gradients [4]. To counteract this effect, we scale the dot products by 1/sqrt(d_k).
4 To illustrate why the dot products get large, assume that the components of q and k are independent random variables with mean 0 and variance 1. Then their dot product, q · k = sum_{i=1}^{d_k} q_i k_i, has mean 0 and variance d_k.
3.2.2 Multi-Head Attention
Instead of performing a single attention function with d_model-dimensional keys, values and queries, we found it beneficial to linearly project the queries, keys and values h times with different, learned linear projections to d_k, d_k and d_v dimensions, respectively. On each of these projected versions of queries, keys and values we then perform the attention function in parallel, yielding d_v-dimensional
4
[End OCR]*