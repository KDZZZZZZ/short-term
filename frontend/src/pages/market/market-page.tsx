import { useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ListBox, Pagination, SearchField, Select } from '@heroui/react'
import { PackageSearch } from 'lucide-react'
import { listProducts } from '@/lib/api/products'
import type { CategoryFilter } from '@/lib/types'
import { CATEGORY_OPTIONS } from '@/lib/format'
import { EmptyState } from '@/components/empty-state'
import { ProductCard, ProductCardSkeleton } from '@/components/product-card'

const PAGE_SIZE = 12

export function MarketPage() {
  const [input, setInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [category, setCategory] = useState<CategoryFilter | null>(null)
  const [page, setPage] = useState(1)

  const { data, isPending, isFetching } = useQuery({
    queryKey: ['products', { keyword, category, page }],
    queryFn: () =>
      listProducts({
        keyword: keyword || undefined,
        category: category ?? undefined,
        page,
        page_size: PAGE_SIZE,
      }),
    placeholderData: keepPreviousData,
  })

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1

  const applySearch = (value: string) => {
    setKeyword(value.trim())
    setPage(1)
  }

  return (
    <div className="flex flex-col gap-6">
      {/* 居中标题 + 副标题（参考 Human Design 设计稿） */}
      <div className="pt-2 text-center">
        <h1 className="text-3xl font-bold tracking-tight text-foreground">校园二手集市</h1>
        <p className="mt-2 text-sm text-muted">让闲置好物，遇见新主人</p>
      </div>

      <div className="mx-auto flex w-full max-w-2xl flex-col gap-3 sm:flex-row sm:items-center">
        <SearchField
          className="w-full sm:max-w-sm"
          name="market-search"
          value={input}
          onChange={setInput}
          onSubmit={applySearch}
          onClear={() => applySearch('')}
        >
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input aria-label="搜索商品标题" placeholder="搜索在售商品…" />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>

        <Select
          className="w-full sm:w-44"
          placeholder="全部分类"
          value={category}
          onChange={(key) => {
            setCategory((key as CategoryFilter | null) ?? null)
            setPage(1)
          }}
        >
          <Select.Trigger>
            <Select.Value />
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover>
            <ListBox>
              {CATEGORY_OPTIONS.map((option) => (
                <ListBox.Item id={option.value} key={option.value} textValue={option.label}>
                  {option.label}
                  <ListBox.ItemIndicator />
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
      </div>

      {isPending ? (
        <div className="grid grid-cols-1 gap-5 min-[480px]:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 8 }, (_, i) => (
            <ProductCardSkeleton key={i} />
          ))}
        </div>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          actionLabel="清除条件"
          actionTo="/"
          description="试试更换关键词或分类，或稍后再来看看"
          icon={<PackageSearch className="size-10" />}
          title={keyword || category ? '没有找到匹配的在售商品' : '市场暂时还没有在售商品'}
        />
      ) : (
        <>
          <div
            className={`grid grid-cols-1 gap-5 transition-opacity min-[480px]:grid-cols-2 lg:grid-cols-3 ${
              isFetching ? 'opacity-60' : ''
            }`}
          >
            {data.items.map((product, index) => (
              <ProductCard animate={{ delay: Math.min(index * 0.03, 0.2) }} key={product.id} product={product} />
            ))}
          </div>

          {totalPages > 1 ? (
            <Pagination className="justify-center">
              <Pagination.Content>
                <Pagination.Item>
                  <Pagination.Previous isDisabled={page === 1} onPress={() => setPage((p) => p - 1)}>
                    <Pagination.PreviousIcon />
                    <span>上一页</span>
                  </Pagination.Previous>
                </Pagination.Item>
                {Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
                  <Pagination.Item key={p}>
                    <Pagination.Link isActive={p === page} onPress={() => setPage(p)}>
                      {p}
                    </Pagination.Link>
                  </Pagination.Item>
                ))}
                <Pagination.Item>
                  <Pagination.Next isDisabled={page === totalPages} onPress={() => setPage((p) => p + 1)}>
                    <span>下一页</span>
                    <Pagination.NextIcon />
                  </Pagination.Next>
                </Pagination.Item>
              </Pagination.Content>
            </Pagination>
          ) : null}
        </>
      )}
    </div>
  )
}
