import { Description, Input, Label, ListBox, Select, TextArea, TextField } from '@heroui/react'
import { CATEGORY_OPTIONS } from '@/lib/format'

export interface ProductFormState {
  title: string
  price: string
  category: string
  description: string
}

export function ProductFormFields({
  form,
  onChange,
}: {
  form: ProductFormState
  onChange: (form: ProductFormState) => void
}) {
  return (
    <>
      <TextField
        className="w-full"
        name="title"
        value={form.title}
        onChange={(value) => onChange({ ...form, title: value })}
      >
        <Label>标题 *</Label>
        <Input maxLength={100} placeholder="一句话描述你的商品" />
      </TextField>

      <div className="grid grid-cols-2 gap-4">
        <TextField
          className="w-full"
          name="price"
          value={form.price}
          onChange={(value) => onChange({ ...form, price: value.trim() })}
        >
          <Label>价格 *</Label>
          <Input className="tabular-nums" inputMode="decimal" placeholder="0.00" />
          <Description>人民币，十进制金额</Description>
        </TextField>

        <Select
          className="w-full"
          placeholder="选择分类"
          value={form.category || null}
          onChange={(key) => onChange({ ...form, category: String(key ?? '') })}
        >
          <Label>分类 *</Label>
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

      <TextField
        className="w-full"
        name="description"
        value={form.description}
        onChange={(value) => onChange({ ...form, description: value })}
      >
        <Label>描述 *</Label>
        <TextArea maxLength={2000} placeholder="成色、使用情况、交易方式等（最多 2000 字）" rows={6} />
      </TextField>
    </>
  )
}
