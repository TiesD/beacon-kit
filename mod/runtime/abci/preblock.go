// SPDX-License-Identifier: MIT
//
// Copyright (c) 2024 Berachain Foundation
//
// Permission is hereby granted, free of charge, to any person
// obtaining a copy of this software and associated documentation
// files (the "Software"), to deal in the Software without
// restriction, including without limitation the rights to use,
// copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following
// conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
// OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
// HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
// WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package abci

import (
	"github.com/berachain/beacon-kit/mod/primitives"
	abcitypes "github.com/berachain/beacon-kit/mod/runtime/abci/types"
	cometabci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// PreBlocker is called by the base app before the block is finalized. It
// is responsible for aggregating oracle data from each validator and writing
// the oracle data to the store.
func (h *Handler) PreBlocker(
	ctx sdk.Context, req *cometabci.RequestFinalizeBlock,
) error {
	logger := ctx.Logger().With("module", "pre-block")
	// Process the Slot.
	if err := h.chainService.ProcessSlot(ctx); err != nil {
		logger.Error("failed to process slot", "error", err)
		return err
	}

	// Extract the beacon block from the ABCI request.
	//
	// TODO: Block factory struct?
	// TODO: Use protobuf and .(type)?
	blk, err := abcitypes.ReadOnlyBeaconBlockFromABCIRequest(
		req,
		h.cfg.BeaconBlockPosition,
		h.chainService.BeaconCfg().ActiveForkVersionForSlot(
			primitives.Slot(req.Height),
		),
	)
	if err != nil {
		return err
	}

	blobSideCars, err := abcitypes.GetBlobSideCars(
		req, h.cfg.BlobSidecarsBlockPosition,
	)
	if err != nil {
		return err
	}

	// Processing the incoming beacon block and blobs.
	cacheCtx, write := ctx.CacheContext()
	if err = h.chainService.ProcessBeaconBlock(
		cacheCtx,
		blk,
		blobSideCars,
	); err != nil {
		logger.Warn(
			"failed to receive beacon block",
			"error",
			err,
		)
		// TODO: Emit Evidence so that the validator can be slashed.
	} else {
		// We only want to persist state changes if we successfully
		// processed the block.
		write()
	}

	// Process the finalization of the beacon block.
	if err = h.chainService.PostBlockProcess(
		ctx, blk,
	); err != nil {
		return err
	}

	// Call the nested child handler.
	return h.callNextPreblockHandler(ctx, req)
}

// callNextHandler calls the next pre-block handler in the chain.
func (h *Handler) callNextPreblockHandler(
	ctx sdk.Context, req *cometabci.RequestFinalizeBlock,
) error {
	// If there is no child handler, we are done, this preblocker
	// does not modify any consensus params so we return an empty
	// response.
	if h.nextPreblock == nil {
		return nil
	}

	return h.nextPreblock(ctx, req)
}